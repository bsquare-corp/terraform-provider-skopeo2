package providerlog

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

// ProviderFormatter ensures that format is actually adhering
// to Terraform Provider conventions
type ProviderFormatter struct {
}

func (p *ProviderFormatter) Format(e *logrus.Entry) ([]byte, error) {
	return []byte(fmt.Sprintf("[DEBUG] %s\n", e.Message)), nil
}
