// Copyright 2015 Apcera Inc. All rights reserved.

package init

var (
	// The setup functions that should be run in order to handle setting up the
	// host system to create and manage containers. These functions focus
	// primarily on runtime actions that must be done each time on boot.
	setupFunctions = []func(*runner) error{
		(*runner).createSystemMounts,
		(*runner).configureEnvironment,
		(*runner).mountCgroups,
		(*runner).loadModules,
		(*runner).configureHostname,
		(*runner).configureNetwork,
		(*runner).displayNetwork,
		(*runner).createDirectories,
		(*runner).readonly,
		(*runner).launchManager,
		(*runner).startInitContainers,
		(*runner).startServer,
	}
)

const (
	// The default location where cgroups should be mounted. This is a constant
	// because it is referenced in multiple functions.
	cgroupsMount = "/sys/fs/cgroup"
)

// defaultConfiguration returns the default codified configuration that is
// applied on boot.
func defaultConfiguration() *kurmaConfig {
	return &kurmaConfig{
		Hostname: "kurmaos",
		Modules:  []string{"e1000"},
		NetworkConfig: &kurmaNetworkConfig{
			Interfaces: []*kurmaNetworkInterface{
				&kurmaNetworkInterface{
					Device:  "lo",
					Address: "127.0.0.1/8",
				},
				&kurmaNetworkInterface{
					Device: "eth.+",
					DHCP:   true,
				},
			},
		},
		Paths: &kurmaPathConfiguration{
			Containers: "/var/kurma/containers",
		},
		ParentCgroupName: "kurma",
		InitContainers: []string{
			// "file:///ntpd.aci",
			// "file:///etcd.aci",
			"file:///gnatsd.aci",
		},
	}
}
