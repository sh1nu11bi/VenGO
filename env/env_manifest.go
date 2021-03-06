/*
   Copyright (C) 2014  Oscar Campos <oscar.campos@member.fsf.org>

   This program is free software; you can redistribute it and/or modify
   it under the terms of the GNU General Public License as published by
   the Free Software Foundation; either version 2 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU General Public License for more details.

   You should have received a copy of the GNU General Public License along
   with this program; if not, write to the Free Software Foundation, Inc.,
   51 Franklin Street, Fifth Floor, Boston, MA 02110-1301 USA.

   See LICENSE file for more details.
*/

package env

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/DamnWidget/VenGO/cache"
	"github.com/DamnWidget/VenGO/utils"
)

// environment manifest structure
type envManifest struct {
	Name      string             `json:"environment_name"`
	GoVersion string             `json:"environment_go_version"`
	Packages  []*packageManifest `json:"environment_packages"`
}

// creates a new envManifest
func NewEnvManifest(env *Environment, options ...func(em *envManifest)) (*envManifest, error) {
	em := new(envManifest)
	for _, option := range options {
		option(em)
	}
	if err := em.getPackages(env); err != nil {
		return nil, err
	}
	return em, nil
}

// detect all the environment manifest packages and populate its own manifests
func (em *envManifest) getPackages(env *Environment) error {
	packages, err := env.Packages(env.VenGO_PATH)
	if err != nil {
		return err
	}
	for _, p := range packages {
		name := func(pm *packageManifest) { pm.Name = p.Name }
		url := func(pm *packageManifest) { pm.Url = p.Url }
		root := func(pm *packageManifest) { pm.Root = p.Root }
		rev := func(pm *packageManifest) { pm.CodeRevision = p.CodeRevision }
		pm, err := NewPackageManifest(env, name, url, root, rev)
		if err != nil {
			return err
		}
		em.Packages = append(em.Packages, pm)
	}

	return nil
}

// generate the environment manifest for exports/import
func (em *envManifest) Generate() ([]byte, error) {
	b, err := json.Marshal(em)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// load an envManifest from a manifest file
func LoadManifest(manifestFile string) (*envManifest, error) {
	file, err := os.Open(manifestFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	var manifest envManifest
	if err := dec.Decode(&manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

// Generate an environment using it's manifest
func (em *envManifest) GenerateEnvironment(v bool, prompt string) error {
	// install go version if it's not installed yet
	if !LookupInstalledVersion(em.GoVersion) {
		if err := cache.CacheDownloadGit(em.GoVersion); err != nil {
			return err
		}
		if err := cache.Compile(em.GoVersion, v, false); err != nil {
			return err
		}
	}
	if prompt == "" {
		prompt = fmt.Sprintf("[%s]", em.Name)
	}
	impEnv := NewEnvironment(em.Name, prompt)
	if err := impEnv.Generate(); err != nil {
		os.RemoveAll(filepath.Join(os.Getenv("VENGO_HOME"), em.Name))
		return err
	}
	if err := impEnv.Install(em.GoVersion); err != nil {
		os.RemoveAll(filepath.Join(os.Getenv("VENGO_HOME"), em.Name))
		return err
	}
	impEnv.activate()
	defer impEnv.deactivate()
	if err := em.installPackages(v); err != nil {
		os.RemoveAll(filepath.Join(os.Getenv("VENGO_HOME"), em.Name))
		return err
	}
	return nil
}

// install all the packages in the manifest using their respective revisions
func (em *envManifest) installPackages(v bool) error {
	curr, _ := os.Getwd()
	environmentPath := filepath.Join(os.Getenv("VENGO_ENV"), "src")
	if err := os.MkdirAll(environmentPath, 0755); err != nil {
		return err
	}
	defer os.Chdir(curr)
	os.Chdir(environmentPath)
	for _, pkg := range em.Packages {
		if pkg.CodeRevision == "0000000000000000000000000000000000000000" {
			continue // we are in a test here
		}
		fmt.Printf("Cloning %s... ", pkg.Name)
		if err := pkg.Vcs.Clone(pkg.Url, pkg.CodeRevision, pkg.Root, v); err != nil {
			fmt.Println(utils.Fail("✖"))
			return err
		}
		fmt.Println(utils.Ok("✔"))
	}
	return nil
}

// lookup for an specific installed go version
func LookupInstalledVersion(version string) bool {
	installed, err := cache.GetInstalled(
		cache.Tags(), cache.AvailableSources(), cache.AvailableBinaries())
	if err != nil {
		log.Fatal(err)
	}

	for _, v := range installed {
		if v == version {
			return true
		}
	}

	return false
}
