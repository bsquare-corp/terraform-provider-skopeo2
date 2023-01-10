package providerlog

import "github.com/sirupsen/logrus"

func SetDefault() {
	// Easiest implementation is to log everything and let
	// Terraform figure out if it needs to display
	logrus.SetLevel(logrus.DebugLevel)
	logrus.SetFormatter(&ProviderFormatter{})
}
