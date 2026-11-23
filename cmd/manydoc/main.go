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
		fmt.Fprintln(stdout, "Usage:\n  manydoc convert -o output.docx|pdf|json|txt [-allow-lossy] input.docx|pdf|json|txt\n  manydoc extract [-format txt|json] [-o output|-] input.docx|pdf\n\nOptions precede the input filename. PDF passwords may be supplied with -password-env NAME.")
		return nil
	}
	command := args[0]
	if command != "convert" && command != "extract" {
		return fmt.Errorf("unknown command %q", command)
	}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.SetOutput(stderr)
	output := fs.String("o", "", "output filename, or - for stdout")
	format := fs.String("format", "txt", "extract output format: txt or json")
	lossy := fs.Bool("allow-lossy", false, "acknowledge imported document fidelity diagnostics")
	passwordEnv := fs.String("password-env", "", "environment variable containing the PDF password")
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
		if outFormat != manydoc.Text && outFormat != manydoc.JSON {
			return errors.New("extract format must be txt or json")
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
	opts := manydoc.ReadOptions{}
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
	write := func(w io.Writer) error {
		return manydoc.Write(ctx, w, doc, outFormat, manydoc.WriteOptions{AllowLossy: *lossy})
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
