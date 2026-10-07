package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
	"uuid"

	"github.com/productdevbook/yuva/api/internal/api"
	"github.com/productdevbook/yuva/api/internal/store"
)

const apiKeyUsage = "usage: yuva api-key create --workspace <id|name> --name <name> | list --workspace <id|name> | revoke <id>"

func apiKey(ctx context.Context, st *store.Store, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return errors.New(apiKeyUsage)
	}
	fs := flag.NewFlagSet("api-key "+args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	workspace := fs.String("workspace", "", "workspace id or exact name")
	switch args[0] {
	case "create":
		name := fs.String("name", "", "name of the key")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(apiKeyUsage)
		}
		ws, err := api.FindWorkspace(ctx, st, *workspace)
		if err != nil {
			return err
		}
		k, secret, err := api.CreateWorkspaceAPIKey(ctx, st, ws.ID, *name)
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "api key %s (%s) created in workspace %s\n", k.ID, k.Prefix, ws.ID)
		_, err = fmt.Fprintln(stdout, secret)
		return err
	case "list":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New(apiKeyUsage)
		}
		ws, err := api.FindWorkspace(ctx, st, *workspace)
		if err != nil {
			return err
		}
		keys, err := st.ListAPIKeys(ctx, ws.ID)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "ID\tPREFIX\tNAME\tCREATED\tLAST USED\tREVOKED")
		for _, k := range keys {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", k.ID, k.Prefix, k.Name, k.CreatedAt.UTC().Format(time.RFC3339),
				optionalTime(k.LastUsedAt), optionalTime(k.RevokedAt))
		}
		return tw.Flush()
	case "revoke":
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New(apiKeyUsage)
		}
		id, err := uuid.Parse(fs.Arg(0))
		if err != nil {
			return fmt.Errorf("%q is not an API key id", fs.Arg(0))
		}
		k, err := api.RevokeAPIKeyByID(ctx, st, id, time.Now())
		if err != nil {
			return err
		}
		fmt.Fprintf(stderr, "api key %s (%s) revoked\n", k.ID, k.Prefix)
		return nil
	default:
		return errors.New(apiKeyUsage)
	}
}

func optionalTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}
