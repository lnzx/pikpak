package cmd

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/lnzx/pikpak/internal/pikpak"
	"github.com/urfave/cli/v3"
)

var FileCmd = &cli.Command{
	Name:    "file",
	Aliases: []string{"f"},
	Usage:   "file manage",
	Commands: []*cli.Command{
		listCmd,
		DownloadCmd,
		deleteCmd,
		clearCmd,
	},
}

var listCmd = &cli.Command{
	Name:        "list",
	Aliases:     []string{"ls"},
	Usage:       "list files in current account",
	ArgsUsage:   "[path | folder-id]",
	Description: "If the argument matches a file/folder ID format (e.g. VOw7XmbR7CNXy-Fk9WWu7cQho2), it is treated as a folder ID; otherwise it is treated as a path (e.g. /My Pack or a folder name).",
	Action: func(ctx context.Context, c *cli.Command) error {
		remotePath := "/"
		if c.Args().Present() {
			remotePath = c.Args().First()
		}
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		files, err := client.ListFiles(ctx, remotePath)
		if err != nil {
			return err
		}
		fmt.Printf("account: %s path: %s\n", acc.Alias, remotePath)
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TYPE\tSIZE\tMODIFIED\tID\tNAME")
		for _, f := range files {
			typ := "file"
			if f.Kind == "drive#folder" {
				typ = "dir"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", typ, pikpak.ByteSize(f.Size), f.ModifiedTime.Format("2006-01-02 15:04:05"), f.ID, f.Name)
		}
		return w.Flush()
	},
}

var deleteCmd = &cli.Command{
	Name:      "delete",
	Aliases:   []string{"del", "rm"},
	Usage:     "Delete files or folders",
	ArgsUsage: "[file-or-folder id ...]",
	Flags: []cli.Flag{
		forceFlag,
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		ids := c.Args().Slice()
		if len(ids) == 0 {
			return fmt.Errorf("must specify at least one file-or-folder id")
		}
		force := c.Bool("force")
		err = client.DeleteFiles(ctx, ids, force)
		if err != nil {
			return err
		}
		fmt.Printf("account: %s delete files force:%v OK\n", acc.Alias, force)
		return nil
	},
}

var clearCmd = &cli.Command{
	Name:  "clear",
	Usage: "move all files to trash",
	Flags: []cli.Flag{
		forceFlag,
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		fs, err := client.ListByParentID(ctx, "")
		if err != nil {
			return err
		}
		if len(fs) == 0 {
			fmt.Println("no files to delete")
			return nil
		}
		var ids []string
		for _, f := range fs {
			if f.Name == pikpak.DefaultFolder && f.Kind == pikpak.FileKindFolder {
				pack, err := client.ListByParentID(ctx, f.ID)
				if err != nil {
					return err
				}
				for _, m := range pack {
					ids = append(ids, m.ID)
				}
			} else {
				ids = append(ids, f.ID)
			}
		}
		if len(ids) == 0 {
			fmt.Println("no files to delete")
			return nil
		}
		force := c.Bool("force")
		if err = client.DeleteFiles(ctx, ids, force); err != nil {
			return err
		}
		fmt.Printf("account: %s clear all files OK\n", acc.Alias)
		return nil
	},
}
