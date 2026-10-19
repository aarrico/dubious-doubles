package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aarrico/dubious-doubles/backend/internal/ingest"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "import-catalog:", err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("import-catalog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	check := flags.Bool("check", false, "validate an offline snapshot without database writes")
	dir := flags.String("snapshot", "", "offline snapshot directory")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("%w; database import is not implemented; use -check -snapshot <dir>", err)
	}
	if !*check || strings.TrimSpace(*dir) == "" || flags.NArg() != 0 {
		return fmt.Errorf("database import is not implemented; use -check -snapshot <dir> with no positional arguments")
	}
	snapshot, err := ingest.LoadSnapshot(*dir)
	if err != nil {
		return fmt.Errorf("check snapshot: %w", err)
	}
	var counts []string
	for _, name := range []string{"pokemon", "moves", "abilities", "items", "battle-states", "types", "natures"} {
		if catalog, ok := snapshot.Catalogs[name]; ok {
			label := name
			if name == "pokemon" {
				label = "Pokémon"
			}
			counts = append(counts, fmt.Sprintf("%d %s records", len(catalog.Records), label))
		}
	}
	_, err = fmt.Fprintf(stdout, "Checked %s; source %s version %s, gameSet %s, locale %s, lockfileSha256 %s. No database writes performed.\n",
		strings.Join(counts, ", "), snapshot.Source.Package, snapshot.Source.Version,
		snapshot.Source.GameSet, snapshot.Source.Locale, snapshot.Source.LockfileSHA256)
	return err
}
