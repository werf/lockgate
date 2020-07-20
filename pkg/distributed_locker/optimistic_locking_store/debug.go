package optimistic_locking_store

import "github.com/werf/lockgate/pkg/util"

func debug(format string, args ...interface{}) {
	util.Debug(format, args...)
}
