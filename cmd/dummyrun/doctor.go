package main

import (
	"errors"
	"flag"
	"fmt"

	"github.com/dummy-run/dummy-run/internal/bidi"
)

func doctorCmd(args []string) int {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	stop := fs.Bool("stop", false, "stop the browsers listed and remove their profile directories")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	l, err := bidi.FindLeftovers()
	if err != nil {
		return fail(err)
	}
	for _, p := range l.Processes {
		fmt.Printf("process %d %s\n", p.PID, p.Args)
	}
	for _, d := range l.Dirs {
		fmt.Printf("profile %s\n", d)
	}
	if len(l.Processes) == 0 && len(l.Dirs) == 0 {
		fmt.Println("no leftover browsers or profile directories")
		return 0
	}
	if !*stop {
		fmt.Printf("%d processes and %d profile directories. Run again with --stop to remove them.\n",
			len(l.Processes), len(l.Dirs))
		return 1
	}
	if err := l.Stop(); err != nil {
		return fail(err)
	}
	fmt.Printf("stopped %d processes and removed %d profile directories\n", len(l.Processes), len(l.Dirs))
	return 0
}
