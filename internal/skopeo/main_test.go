package skopeo

import (
	"os"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestMain(m *testing.M) {

	logrus.SetLevel(logrus.DebugLevel)
	exitVal := m.Run()

	os.Exit(exitVal)
}
