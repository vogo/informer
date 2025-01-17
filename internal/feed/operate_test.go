package feed

import (
	"github.com/vogo/vogo/vos"
	"testing"
)

func TestOperate(t *testing.T) {
	exeDir := vos.EnvString("LOCAL_FEED_DB_DIR")
	if exeDir == "" {
		t.Skip("LOCAL_FEED_DB_DIR is empty")
		return
	}
	InitFeedDB(exeDir)

	parseSource("14")
}
