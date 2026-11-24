package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/artpar/manydoc"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "manydoc:", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(stdout, "Usage:\n  manydoc convert -o output.docx|pdf|sf|sf.gz|json|txt [-allow-lossy] input.docx|pdf|sf|sf.gz|json|txt\n  manydoc extract [-format txt|sf|sf.gz|json] [-o output|-] input.docx|pdf\n\nOptions precede the input filename. PDF passwords may be supplied with -password-env NAME.\nPDF fonts: -pdf-font Family[/B|I|BI]=path.ttf and -pdf-font-map Requested=Target (repeatable).")
		return nil
	}
	command := args[0]
	if command != "convert" && command != "extract" {
		return fmt.Errorf("unknown command %q", command)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	output := fs.String("o", "", "output filename, or - for stdout")
	format := fs.String("format", "txt", "extract output format: txt, sf, sf.gz or json")
	lossy := fs.Bool("allow-lossy", false, "acknowledge imported document fidelity diagnostics")
	passwordEnv := fs.String("password-env", "", "environment variable containing the PDF password")
	pdfSkipInference := fs.Bool("pdf-skip-inference", false, "retain untagged PDF paint order instead of inferring geometric structure")
	var fontMaps, fontFiles repeatedFlag
	fs.Var(&fontMaps, "pdf-font-map", "explicit requested=target font family mapping (repeatable)")
	fs.Var(&fontFiles, "pdf-font", "family[/B|I|BI]=path.ttf (repeatable)")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("exactly one input filename is required; flags precede the filename")
	}
	inName := fs.Arg(0)
	inFormat, err := manydoc.FormatFromName(inName)
	if err != nil {
		return err
	}
	var outFormat manydoc.Format
	if command == "extract" {
		outFormat = manydoc.Format(*format)
		if outFormat != manydoc.Text && outFormat != manydoc.JSON && outFormat != manydoc.SF && outFormat != manydoc.SFGzip {
			return errors.New("extract format must be txt, sf, sf.gz or json")
		}
		if *output == "" {
			*output = "-"
		}
	} else {
		if *output == "" || *output == "-" {
			return errors.New("convert requires an output filename with a supported extension")
		}
		outFormat, err = manydoc.FormatFromName(*output)
		if err != nil {
			return err
		}
	}
	opts := manydoc.ReadOptions{PDFSkipInference: *pdfSkipInference}
	if *passwordEnv != "" {
		var ok bool
		opts.Password, ok = os.LookupEnv(*passwordEnv)
		if !ok {
			return errors.New("password environment variable is not set")
		}
	}
	in, err := os.Open(inName)
	if err != nil {
		return err
	}
	defer in.Close()
	doc, err := manydoc.Read(ctx, in, inFormat, opts)
	if err != nil {
		return err
	}
	for _, d := range doc.Diagnostics {
		fmt.Fprintf(stderr, "%s: %s\n", d.Code, d.Message)
	}
	writeOpts := manydoc.WriteOptions{AllowLossy: *lossy, PDFFontMap: map[string]string{}}
	for _, value := range fontMaps {
		from, to, ok := strings.Cut(value, "=")
		if !ok || from == "" || to == "" {
			return errors.New("pdf-font-map requires requested=target")
		}
		if _, ok := writeOpts.PDFFontMap[from]; ok {
			return errors.New("duplicate font mapping")
		}
		writeOpts.PDFFontMap[from] = to
	}
	for _, value := range fontFiles {
		family, path, ok := strings.Cut(value, "=")
		if !ok || family == "" || path == "" {
			return errors.New("pdf-font requires family[/style]=path.ttf")
		}
		style := ""
		if i := strings.LastIndex(family, "/"); i >= 0 {
			style = family[i+1:]
			family = family[:i]
		}
		if style != "" && style != "B" && style != "I" && style != "BI" {
			return errors.New("invalid PDF font style")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		writeOpts.PDFFonts = append(writeOpts.PDFFonts, manydoc.FontFace{Family: family, Bold: strings.Contains(style, "B"), Italic: strings.Contains(style, "I"), Data: data})
	}
	write := func(w io.Writer) error {
		return manydoc.Write(ctx, w, doc, outFormat, writeOpts)
	}
	if *output == "-" {
		return write(stdout)
	}
	// Rename only after successful conversion so a failed operation preserves an
	// existing destination and never exposes a partial generated document.
	f, err := os.CreateTemp(filepath.Dir(*output), ".manydoc-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := write(f); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), *output)
}

type repeatedFlag []string

func (v *repeatedFlag) String() string     { return strings.Join(*v, ",") }
func (v *repeatedFlag) Set(s string) error { *v = append(*v, s); return nil }
