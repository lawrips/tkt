package ticket

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// GenerateID creates a short ticket ID and avoids collisions in dir.
func GenerateID(dir string) (string, error) {
	for i := 0; i < 64; i++ {
		id, err := randomID()
		if err != nil {
			return "", err
		}
		path := filepath.Join(dir, id+".md")
		_, err = os.Stat(path)
		if os.IsNotExist(err) {
			return id, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("unable to generate unique ticket id")
}

func randomID() (string, error) {
	buf := make([]byte, 2)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "t-" + hex.EncodeToString(buf), nil
}
