package cli

import "github.com/lawrips/tkt/internal/engine"

func centralStoreRootDir() (string, error) {
	return engine.CentralStoreRoot()
}

func centralProjectDir(projectName string) (string, error) {
	return engine.CentralProjectDir(projectName)
}
