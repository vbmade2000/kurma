// Copyright 2015 Apcera Inc. All rights reserved.

package container

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	kschema "github.com/apcera/kurma/schema"
	"github.com/apcera/kurma/stage1"
	"github.com/apcera/kurma/util/cgroups"
	"github.com/apcera/logray"
	"github.com/apcera/util/uuid"
	"github.com/appc/spec/schema"
	"github.com/appc/spec/schema/types"
)

// Options contains settings that are used by the Container Manager and
// Containers running on the host.
type Options struct {
	ParentCgroupName   string
	ContainerDirectory string
	VolumeDirectory    string
	RequiredNamespaces []string
}

// Manager handles the management of the containers running and available on the
// current host.
type Manager struct {
	Log     *logray.Logger
	Options *Options

	imageManager   stage1.ImageManager
	networkManager stage1.NetworkManager
	cgroup         *cgroups.Cgroup
	volumeLock     sync.Mutex

	containers     map[string]*Container
	containersLock sync.RWMutex

	HostSocketFile string
}

// NewManager creates a new Manager with the provided options. It will ensure
// the manager is setup and ready to create containers with the provided
// configuration.
func NewManager(imageManager stage1.ImageManager, networkManager stage1.NetworkManager, opts *Options) (stage1.ContainerManager, error) {
	// validate cgroups is properly setup on the host
	if err := cgroups.CheckCgroups(); err != nil {
		return nil, fmt.Errorf("failed to check cgroups: %v", err)
	}

	// create the parent cgroup for all child containers to be in
	cg, err := cgroups.New(opts.ParentCgroupName)
	if err != nil {
		return nil, err
	}

	m := &Manager{
		Log:            logray.New(),
		Options:        opts,
		imageManager:   imageManager,
		networkManager: networkManager,
		containers:     make(map[string]*Container),
		cgroup:         cg,
	}
	return m, nil
}

// SetLog sets the logger to be used by the manager.
func (manager *Manager) SetLog(log *logray.Logger) {
	manager.Log = log
}

// SetHostSocketFile sets the path to the host's socket file for granting API
// access.
func (manager *Manager) SetHostSocketFile(hostSocketFile string) {
	manager.HostSocketFile = hostSocketFile
}

func (manager *Manager) SetNetworkManager(networkManager stage1.NetworkManager) {
	manager.networkManager = networkManager
}

// Validate will ensure that the image manifest provided is valid to be run on
// the system. It will return nil if it is valid, or will return an error if
// something is invalid.
func (manager *Manager) Validate(imageManifest *schema.ImageManifest) error {
	if imageManifest.App == nil {
		return fmt.Errorf("the manifest must specify an App")
	}

	// If the namespaces isolator is specified, validate a minimum set of namespaces
	if iso := imageManifest.App.Isolators.GetByName(kschema.LinuxNamespacesName); iso != nil {
		if niso, ok := iso.Value().(*kschema.LinuxNamespaces); ok {
			checks := map[string]func() kschema.LinuxNamespaceValue{
				"ipc":  niso.IPC,
				"net":  niso.Net,
				"pid":  niso.PID,
				"user": niso.User,
				"uts":  niso.UTS,
			}
			for _, ns := range manager.Options.RequiredNamespaces {
				f, exists := checks[ns]
				if !exists {
					return fmt.Errorf("Internal server error")
				}
				if f() == kschema.LinuxNamespaceHost {
					return fmt.Errorf("the manifest %s isolator must require the %s namespace", kschema.LinuxNamespacesName, ns)
				}
			}
		}
	}

	return nil
}

// Create begins launching a container with the provided image manifest and
// reader as the source of the ACI.
func (manager *Manager) Create(
	name string, imageManifest *schema.ImageManifest, imageHash string,
) (stage1.Container, error) {
	// revalidate the image
	if err := manager.Validate(imageManifest); err != nil {
		return nil, err
	}

	hash, err := types.NewHash(imageHash)
	if err != nil {
		return nil, err
	}

	// handle a blank name
	if name == "" {
		n, err := convertACIdentifierToACName(imageManifest.Name)
		if err != nil {
			return nil, err
		}
		name = n.String()
	}

	// populate the container
	container := &Container{
		manager:   manager,
		log:       manager.Log.Clone(),
		uuid:      uuid.Variant4().String(),
		waitch:    make(chan bool),
		imageHash: imageHash,
		image:     imageManifest,
		pod:       kschema.BlankPodManifest(),
	}
	container.pod.Annotations = imageManifest.Annotations
	container.pod.Apps = schema.AppList([]schema.RuntimeApp{
		schema.RuntimeApp{
			Name: types.ACName(name),
			App:  imageManifest.App,
			Image: schema.RuntimeImage{
				ID:     *hash,
				Name:   &imageManifest.Name,
				Labels: imageManifest.Labels,
			},
		},
	})
	container.log.SetField("container", container.uuid)
	container.log.Debugf("Launching container %s", container.uuid)

	// add it to the manager's map
	manager.containersLock.Lock()
	manager.containers[container.uuid] = container
	manager.containersLock.Unlock()

	// begin the startup sequence
	container.start()

	return container, nil
}

// removes a child container from the Container Manager.
func (manager *Manager) remove(container *Container) {
	manager.containersLock.Lock()
	container.mutex.Lock()
	delete(manager.containers, container.uuid)
	container.mutex.Unlock()
	manager.containersLock.Unlock()
}

// Containers returns a slice of the current containers on the host.
func (manager *Manager) Containers() []stage1.Container {
	manager.containersLock.RLock()
	defer manager.containersLock.RUnlock()
	containers := make([]stage1.Container, 0, len(manager.containers))
	for _, c := range manager.containers {
		containers = append(containers, c)
	}
	return containers
}

// Container returns a specific container matching the provided UUID, or nil if
// a container with the UUID does not exist.
func (manager *Manager) Container(uuid string) stage1.Container {
	manager.containersLock.RLock()
	defer manager.containersLock.RUnlock()
	return manager.containers[uuid]
}

// SwapDirectory can be used to temporarily use a different container path for
// an operation. This is a temporary hack util a Container object can specify
// its own path.
func (manager *Manager) SwapDirectory(containerDirectory string, f func()) {
	dir := manager.Options.ContainerDirectory
	manager.Options.ContainerDirectory = containerDirectory
	defer func() { manager.Options.ContainerDirectory = dir }()
	f()
}

// getVolumePath will get the absolute path on the host to the named volume. It
// will also ensure that the volume name exists within the volumes directory.
func (manager *Manager) getVolumePath(name string) (string, error) {
	if !types.ValidACName.MatchString(name) {
		return "", fmt.Errorf("invalid characters present in volume name")
	}

	volumePath := filepath.Join(manager.Options.VolumeDirectory, name)

	manager.volumeLock.Lock()
	defer manager.volumeLock.Unlock()

	if err := os.Mkdir(volumePath, os.FileMode(0755)); err != nil && !os.IsExist(err) {
		return "", err
	}
	return volumePath, nil
}
