// Copyright 2016 Apcera Inc. All rights reserved.

package network

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"text/template"

	"github.com/apcera/kurma/networking/types"
	"github.com/apcera/kurma/stage1"
)

func (d *networkDriver) generateInterfaceName(container stage1.Container, interfaces []*types.IPResult) (string, error) {
	funcsMap := funcsForContainer(container, interfaces)

	// parse the template
	tmpl, err := template.New("interface").Funcs(funcsMap).Parse(d.config.ContainerInterface)
	if err != nil {
		return "", err
	}

	// execute the template and return
	buffer := bytes.NewBufferString("")
	if err := tmpl.Execute(buffer, nil); err != nil {
		return "", err
	}

	cint := buffer.String()

	for _, res := range interfaces {
		if res.ContainerInterface == cint {
			return "", fmt.Errorf("interface %q is not unique on the container", cint)
		}
	}

	return cint, nil
}

const alphaNumericalCharacters = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func funcsForContainer(container stage1.Container, interfaces []*types.IPResult) template.FuncMap {
	return template.FuncMap{
		"uuid":      container.UUID,
		"shortuuid": func() string { return container.UUID()[:8] },
		"num":       func() int { return len(interfaces) },
		"random":    random,
	}
}

func random(n int) string {
	// enforce a max limit for some sanity
	if n > 32 {
		n = 32
	}

	// generate some random data, then iterate them and limit to within our
	// allowed character
	b := make([]byte, n)
	rand.Read(b)
	for i, c := range b {
		b[i] = alphaNumericalCharacters[c%byte(len(alphaNumericalCharacters))]
	}
	return string(b)
}
