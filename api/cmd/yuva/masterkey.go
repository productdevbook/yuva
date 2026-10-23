package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/productdevbook/yuva/api/internal/api"
)

const masterKeyUsage = "usage: yuva master-key rotate [--batch <n>]\n" +
	"       yuva master-key check [--batch <n>]"

const masterKeyListed = 20

func masterKeyCommand(ctx context.Context, srv *api.Server, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || (args[0] != "rotate" && args[0] != "check") {
		return errors.New(masterKeyUsage)
	}
	fs := flag.NewFlagSet("master-key "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	batch := fs.Int("batch", 500, "rows read and re-sealed per query")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 || *batch < 1 || *batch > 10000 {
		return errors.New(masterKeyUsage)
	}
	rotate := args[0] == "rotate"
	progress := func(r api.SealedReport) {
		if rotate {
			fmt.Fprintf(stdout, "%s: %d re-sealed, %d already under the current key, %d unreadable\n", r.Column, r.Previous, r.Current, len(r.Unreadable))
		} else {
			fmt.Fprintf(stdout, "%s: %d under the current key, %d under the previous key, %d unreadable\n", r.Column, r.Current, r.Previous, len(r.Unreadable))
		}
		for i, u := range r.Unreadable {
			if i == masterKeyListed {
				fmt.Fprintf(stdout, "  … and %d more\n", len(r.Unreadable)-i)
				break
			}
			fmt.Fprintf(stdout, "  unreadable: %s\n", u)
		}
	}
	var rep api.MasterKeyReport
	var err error
	if rotate {
		rep, err = srv.RotateMasterKey(ctx, *batch, progress)
	} else {
		rep, err = srv.CheckMasterKey(ctx, *batch, progress)
	}
	if err != nil {
		return err
	}
	if rep.UnkeptOptOutKeys > 0 {
		fmt.Fprintf(stdout, "workspace opt-out keys not kept yet: %d (yuva master-key rotate keeps them)\n", rep.UnkeptOptOutKeys)
	}
	unreadable := 0
	for _, c := range rep.Columns {
		unreadable += len(c.Unreadable)
	}
	switch {
	case unreadable > 0:
		return fmt.Errorf("%d values open with neither master key: set them again (the listed rows) or delete them; nothing else can read them", unreadable)
	case rep.UnkeptOptOutKeys > 0 || (!rotate && !rep.Clean()):
		return errors.New("not everything is under the current master key yet: keep YUVA_MASTER_KEY_PREVIOUS and run yuva master-key rotate")
	}
	if rotate {
		fmt.Fprintln(stdout, "everything is sealed under the current master key; run yuva master-key check, then remove YUVA_MASTER_KEY_PREVIOUS once links in mail sent before the switch may stop working")
	} else {
		fmt.Fprintln(stdout, "everything opens with the current master key")
	}
	return nil
}
