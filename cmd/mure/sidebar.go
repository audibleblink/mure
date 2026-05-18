package main

import (
	"context"
	"fmt"
	"os"

	"github.com/audibleblink/mure/internal/sidebar"
)

func cmdSidebar(ctx context.Context, _ []string, _, stderr *os.File) int {
	// The sidebar pane is split with `-c '#{pane_current_path}'`, so our
	// cwd is already that of the invoking pane (where prefix+M was hit).
	if err := sidebar.Run(ctx); err != nil {
		fmt.Fprintln(stderr, "mure sidebar:", err)
		return 1
	}
	return 0
}
