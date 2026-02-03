package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/dnsoa/iplist"
)

func exportCmd(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	dbPath := fs.String("db", "iplist.db", "db file")
	what := fs.String("what", "", "export table: country|cn_province|cn_city|provider")
	outPath := fs.String("out", "-", "output file ('-' for stdout)")
	_ = fs.Parse(args)
	if *what == "" {
		fatal(fmt.Errorf("export: need -what country|cn_province|cn_city|provider"))
	}

	db, err := iplist.Open(*dbPath)
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	w, closeFn, err := openOut(*outPath)
	if err != nil {
		fatal(err)
	}
	defer closeFn()

	switch *what {
	case "country":
		err = db.ExportCountryTSV(w)
	case "cn_province":
		err = db.ExportCNProvinceTSV(w)
	case "cn_city":
		err = db.ExportCNCityTSV(w)
	case "provider":
		err = db.ExportProviderTSV(w)
	default:
		fatal(fmt.Errorf("export: unknown -what %q", *what))
	}
	if err != nil {
		fatal(err)
	}
}

func openOut(path string) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return os.Stdout, func() error { return nil }, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
