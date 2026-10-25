// Command coverfloor fails when any package's statement coverage is below a floor.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

func main() {
	profile := flag.String("profile", "coverage.out", "coverage profile")
	min := flag.Float64("min", 80, "minimum percent per package")
	flag.Parse()

	f, err := os.Open(*profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	defer func() { _ = f.Close() }()

	// With -coverpkg every test binary reports every block: merge per block.
	stmts, hit := map[string]int{}, map[string]bool{}
	sc := bufio.NewScanner(f)

	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}

		file, rest, _ := strings.Cut(line, ":")
		fields := strings.Fields(rest)

		if len(fields) != 3 {
			continue
		}

		n, _ := strconv.Atoi(fields[1])
		c, _ := strconv.Atoi(fields[2])
		block := file + ":" + fields[0]
		stmts[block] = n

		if c > 0 {
			hit[block] = true
		}
	}

	total, covered := map[string]int{}, map[string]int{}

	for block, n := range stmts {
		file, _, _ := strings.Cut(block, ":")
		pkg := path.Dir(file)
		total[pkg] += n

		if hit[block] {
			covered[pkg] += n
		}
	}

	pkgs := make([]string, 0, len(total))
	for p := range total {
		pkgs = append(pkgs, p)
	}

	sort.Strings(pkgs)

	failed := false

	for _, p := range pkgs {
		pct := 100 * float64(covered[p]) / float64(total[p])
		mark := "ok  "

		if pct < *min {
			mark, failed = "FAIL", true
		}

		fmt.Printf("%s %5.1f%% %s\n", mark, pct, p)
	}

	if failed {
		os.Exit(1)
	}
}
