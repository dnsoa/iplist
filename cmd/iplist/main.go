package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dnsoa/iplist"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	sub := os.Args[1]
	switch sub {
	case "build":
		buildCmd(os.Args[2:])
	case "lookup":
		lookupCmd(os.Args[2:])
	case "cloud":
		cloudCmd(os.Args[2:])
	case "provider":
		providerCmd(os.Args[2:])
	case "export":
		exportCmd(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  iplist build   -data ./data -out ./iplist.db")
	fmt.Fprintln(os.Stderr, "  iplist lookup  -db ./iplist.db 1.2.3.4")
	fmt.Fprintln(os.Stderr, "  iplist cloud   -db ./iplist.db aliyun")
	fmt.Fprintln(os.Stderr, "  iplist provider -db ./iplist.db chinatelecom")
	fmt.Fprintln(os.Stderr, "  iplist export  -db ./iplist.db -what country|cn_province|cn_city|provider -out -")
}

func buildCmd(args []string) {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	dataDir := fs.String("data", "data", "data directory")
	out := fs.String("out", "iplist.db", "output db file")
	_ = fs.Parse(args)

	if err := iplist.Build(*dataDir, *out); err != nil {
		fatal(err)
	}
}

func lookupCmd(args []string) {
	fs := flag.NewFlagSet("lookup", flag.ExitOnError)
	dbPath := fs.String("db", "iplist.db", "db file")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fatal(fmt.Errorf("lookup: need 1 ip"))
	}

	db, err := iplist.Open(*dbPath)
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	res, ok, err := db.Lookup(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	if !ok {
		fmt.Println("no match")
		return
	}

	fmt.Printf("ip=%s\n", res.IP)
	if res.CountryCode != "" {
		fmt.Printf("country=%s (%s)\n", res.CountryCode, res.CountryName)
	}
	if res.CNCityCode != "" {
		fmt.Printf("cn_city=%s (%s)\n", res.CNCityCode, res.CNCityName)
	} else if res.CNProvinceCode != "" {
		fmt.Printf("cn_province=%s (%s)\n", res.CNProvinceCode, res.CNProvinceName)
	}
	if res.ProviderKey != "" {
		fmt.Printf("provider=%s (%s) kind=%d\n", res.ProviderKey, res.ProviderName, res.ProviderKind)
	}
}

func cloudCmd(args []string) {
	fs := flag.NewFlagSet("cloud", flag.ExitOnError)
	dbPath := fs.String("db", "iplist.db", "db file")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fatal(fmt.Errorf("cloud: need 1 vendor key, e.g. aliyun"))
	}

	db, err := iplist.Open(*dbPath)
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	cidrs, err := db.CloudIPs(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	for _, c := range cidrs {
		fmt.Println(c)
	}
}

func providerCmd(args []string) {
	fs := flag.NewFlagSet("provider", flag.ExitOnError)
	dbPath := fs.String("db", "iplist.db", "db file")
	_ = fs.Parse(args)
	if fs.NArg() != 1 {
		fatal(fmt.Errorf("provider: need 1 provider key"))
	}

	db, err := iplist.Open(*dbPath)
	if err != nil {
		fatal(err)
	}
	defer db.Close()

	cidrs, kind, err := db.ProviderIPs(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	_ = kind
	for _, c := range cidrs {
		fmt.Println(c)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
