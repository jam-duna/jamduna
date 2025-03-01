package log

import (
	"fmt"
	"log/syslog"
	"testing"
)

func TestSyslog(t *testing.T) {
	mockServerAddr := "dev.jamduna.org:5000"
	logger, err := syslog.Dial("tcp", mockServerAddr, syslog.LOG_INFO, "jamduna")
	if err != nil {
		t.Fatalf("Failed to create syslog client: %v", err)
	}
	defer logger.Close()

	testMessage := "jamduna|1|2|3"
	if err := logger.Info(testMessage); err != nil {
		t.Fatalf("Failed to send syslog message: %v", err)
	}

	fmt.Printf("Successfully sent syslog message to %s", mockServerAddr)
}
