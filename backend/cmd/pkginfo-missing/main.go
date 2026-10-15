// Command pkginfo-missing lists the packages of an apt index that have no
// curated entry in internal/pkginfo (docs/APPSHUB_PLAN.md, App11), with the
// category the name convention gives them and their own Description, so the
// entries file can be completed once the box (or its feed) is reachable:
//
//	go run ./cmd/pkginfo-missing Packages        # a feed's Packages file
//	go run ./cmd/pkginfo-missing Packages.gz
//	apt-cache dumpavail | go run ./cmd/pkginfo-missing -
//
// The output is tab separated: name, category id, description.
package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/antonbier/DreamHub/backend/internal/apt"
	"github.com/antonbier/DreamHub/backend/internal/pkginfo"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: pkginfo-missing <Packages file | Packages.gz | ->")
		os.Exit(2)
	}
	data, err := read(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "pkginfo-missing:", err)
		os.Exit(1)
	}
	missing, total := list(data)
	for _, p := range missing {
		fmt.Printf("%s\t%s\t%s\n", p.Name, pkginfo.CategoryOf(p.Name), p.Description)
	}
	fmt.Fprintf(os.Stderr, "%d of %d packages have no entry\n", len(missing), total)
}

// list returns the packages in a Packages index without a curated entry,
// and how many the index holds.
func list(index []byte) (missing []apt.Package, total int) {
	pkgs := apt.ParseDumpavail(index)
	names := make([]string, len(pkgs))
	for i, p := range pkgs {
		names[i] = p.Name
	}
	no := map[string]bool{}
	for _, n := range pkginfo.Missing(names) {
		no[n] = true
	}
	for _, p := range pkgs {
		if no[p.Name] {
			p.Description = strings.TrimSpace(p.Description)
			missing = append(missing, p)
		}
	}
	return missing, len(pkgs)
}

func read(path string) ([]byte, error) {
	var r io.Reader = os.Stdin
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
		if strings.HasSuffix(path, ".gz") {
			if r, err = gzip.NewReader(f); err != nil {
				return nil, err
			}
		}
	}
	return io.ReadAll(r)
}
