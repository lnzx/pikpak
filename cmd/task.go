package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/urfave/cli/v3"
)

var TaskCmd = &cli.Command{
	Name:    "task",
	Aliases: []string{"t"},
	Usage:   "manage offline tasks",
	Commands: []*cli.Command{
		taskAddCmd,
		taskListCmd,
		taskDeleteCmd,
		taskClearCmd,
	},
}

var taskAddCmd = &cli.Command{
	Name:      "add",
	Aliases:   []string{"a"},
	Usage:     "create offline download task",
	ArgsUsage: "[url...]",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "input",
			Aliases: []string{"i"},
			Usage:   "file with URLs, one per line",
		},
		&cli.StringFlag{
			Name:    "folder",
			Aliases: []string{"f"},
			Usage:   "destination folder: path (e.g. /movies/2024) or folder-id",
		},
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		urls := c.Args().Slice()
		if input := c.String("input"); input != "" {
			lines, err := readLines(input)
			if err != nil {
				return err
			}
			urls = append(urls, lines...)
		}
		if len(urls) == 0 {
			return errors.New("task add requires url or -i file")
		}
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		parentID := ""
		if folder := c.String("folder"); folder != "" {
			if strings.Contains(folder, "/") {
				parentID, err = client.FolderIDByPath(ctx, folder)
				if err != nil {
					return err
				}
			} else {
				parentID = folder
			}
		}
		submitted, failures := 0, 0
		for _, rawURL := range urls {
			if err := ctx.Err(); err != nil {
				return err
			}
			rawURL = strings.TrimSpace(rawURL)
			if rawURL == "" {
				continue
			}
			task, err := client.CreateOfflineTask(ctx, rawURL, parentID)
			if err != nil {
				failures++
				fmt.Printf("failed\taccount=%s\turl=%s\terror=%v\n", acc.Alias, rawURL, err)
				continue
			}
			submitted++
			name := task.Name
			if name == "" {
				name = task.FileName
			}
			fmt.Printf("submitted\taccount=%s\ttask_id=%s\tphase=%s\tname=%s\n", acc.Alias, task.ID, task.Phase, name)
		}
		if failures > 0 {
			return fmt.Errorf("submitted %d task(s), failed %d task(s)", submitted, failures)
		}
		return nil
	},
}

var taskListCmd = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "list remote offline tasks",
	Action: func(ctx context.Context, c *cli.Command) error {
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		tasks, err := client.OfflineTasks(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("account: %s\n", acc.Alias)
		if len(tasks) == 0 {
			fmt.Println("no tasks")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "TASK_ID\tPHASE\tPROGRESS\tFILE_ID\tNAME\tMESSAGE")
		for _, task := range tasks {
			name := task.FileName
			if name == "" {
				name = task.Name
			}
			fmt.Fprintf(w, "%s\t%s\t%d%%\t%s\t%s\t%s\n", task.ID, task.Phase, task.Progress, task.FileID, name, task.Message)
		}
		return w.Flush()
	},
}

var taskDeleteCmd = &cli.Command{
	Name:      "delete",
	Aliases:   []string{"del", "rm"},
	Usage:     "delete offline tasks",
	UsageText: "pikpak task delete [--delete-files] [task-id ...]",
	Flags: []cli.Flag{
		deleteFilesFlag,
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		deleteFiles := c.Bool("delete-files")
		ids := c.Args().Slice()
		if len(ids) == 0 {
			return fmt.Errorf("must specify at least one task id")
		}
		if err := client.DeleteTasks(ctx, deleteFiles, ids); err != nil {
			return err
		}
		fmt.Printf("account: %s delete tasks deleteFiles:%v OK\n", acc.Alias, deleteFiles)
		return nil
	},
}

var taskClearCmd = &cli.Command{
	Name:  "clear",
	Usage: "clear offline tasks",
	Flags: []cli.Flag{
		deleteFilesFlag,
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		client, acc, err := clientFromContext(ctx, c)
		if err != nil {
			return err
		}
		deleteFiles := c.Bool("delete-files")
		if err := client.ClearTasks(ctx, deleteFiles); err != nil {
			return err
		}
		fmt.Printf("account: %s clear tasks --delete-files:%v OK\n", acc.Alias, deleteFiles)
		return nil
	},
}
