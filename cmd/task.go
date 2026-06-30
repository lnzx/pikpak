package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/lnzx/pikpak/internal/pikpak"
	"github.com/lnzx/pikpak/internal/pool"

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

		p, err := poolFromContext(ctx, c)
		if err != nil {
			return err
		}

		// Single-account mode: -a was specified, or only one account exists.
		if p == nil {
			client, acc, err := clientFromContext(ctx, c)
			if err != nil {
				return err
			}
			folderID, err := resolveFolder(ctx, client, c.String("folder"))
			if err != nil {
				return err
			}
			state, err := pool.LoadState()
			if err != nil {
				return err
			}
			submitted, failures := submitTasks(ctx, client, acc.Alias, urls, folderID)
			for _, id := range submitted {
				state.AddTask(acc.Alias, id)
			}
			if err := pool.SaveState(state); err != nil {
				return err
			}
			if failures > 0 {
				return fmt.Errorf("submitted %d task(s), failed %d task(s)", len(submitted), failures)
			}
			return nil
		}

		// Multi-account mode: auto-select the best account.
		acc, err := p.SelectForAdd(ctx)
		if err != nil {
			return err
		}
		client, err := p.ClientFor(ctx, acc)
		if err != nil {
			return err
		}
		folderID, err := resolveFolder(ctx, client, c.String("folder"))
		if err != nil {
			return err
		}
		state, err := pool.LoadState()
		if err != nil {
			return err
		}
		submitted, failures := submitTasks(ctx, client, acc.Alias, urls, folderID)
		// Optimistic quota update.
		if as := state.GetOrCreate(acc.Alias); as.QuotaCache != nil {
			as.QuotaCache.CloudDownloadUsage += int64(len(submitted))
		}
		for _, id := range submitted {
			state.AddTask(acc.Alias, id)
		}
		if err := pool.SaveState(state); err != nil {
			return err
		}
		if failures > 0 {
			return fmt.Errorf("submitted %d task(s), failed %d task(s)", len(submitted), failures)
		}
		return nil
	},
}

// submitTasks creates an offline task for each URL and prints progress.
// Returns the task IDs that were successfully submitted.
func submitTasks(ctx context.Context, client *pikpak.Client, alias string, urls []string, parentID string) (submitted []string, failures int) {
	for _, rawURL := range urls {
		if err := ctx.Err(); err != nil {
			return
		}
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		task, err := client.CreateOfflineTask(ctx, rawURL, parentID)
		if err != nil {
			failures++
			fmt.Printf("failed\taccount=%s\turl=%s\terror=%v\n", alias, rawURL, err)
			continue
		}
		submitted = append(submitted, task.ID)
		name := task.Name
		if name == "" {
			name = task.FileName
		}
		fmt.Printf("submitted\taccount=%s\ttask_id=%s\tphase=%s\tname=%s\n", alias, task.ID, task.Phase, name)
	}
	return
}

// resolveFolder returns the parent folder ID, or "" when no folder flag is set.
func resolveFolder(ctx context.Context, client *pikpak.Client, folder string) (string, error) {
	if folder == "" {
		return "", nil
	}
	if pikpak.IsFileID(folder) {
		return folder, nil
	}
	return client.FolderIDByPath(ctx, folder)
}

var taskListCmd = &cli.Command{
	Name:    "list",
	Aliases: []string{"ls"},
	Usage:   "list remote offline tasks",
	Action: func(ctx context.Context, c *cli.Command) error {
		p, err := poolFromContext(ctx, c)
		if err != nil {
			return err
		}

		// Single-account mode.
		if p == nil {
			client, acc, err := clientFromContext(ctx, c)
			if err != nil {
				return err
			}
			tasks, err := client.OfflineTasks(ctx)
			if err != nil {
				return err
			}
			fmt.Printf("account: %s\n", acc.Alias)
			printTaskTable(tasks)
			// Sync local task_ids with remote reality.
			syncTaskState(acc.Alias, tasks)
			return nil
		}

		// Multi-account mode: iterate all accounts.
		clients, accounts, err := p.ClientsForAll(ctx)
		if err != nil {
			return err
		}
		for i, client := range clients {
			tasks, err := client.OfflineTasks(ctx)
			if err != nil {
				fmt.Printf("account: %s error=%v\n", accounts[i].Alias, err)
				continue
			}
			fmt.Printf("account: %s\n", accounts[i].Alias)
			printTaskTable(tasks)
			// Sync local task_ids with remote reality.
			syncTaskState(accounts[i].Alias, tasks)
			if i < len(clients)-1 {
				fmt.Println()
			}
		}
		return nil
	},
}

func printTaskTable(tasks []pikpak.OfflineTask) {
	if len(tasks) == 0 {
		fmt.Println("no tasks")
		return
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
	w.Flush()
}

// syncTaskState rebuilds the local task_ids for alias from the remote task list.
// Quota cache is preserved.
func syncTaskState(alias string, tasks []pikpak.OfflineTask) {
	state, err := pool.LoadState()
	if err != nil {
		return // best-effort, don't fail the command
	}
	ids := make([]string, 0, len(tasks))
	for _, t := range tasks {
		ids = append(ids, t.ID)
	}
	as := state.GetOrCreate(alias)
	as.TaskIDs = ids
	pool.SaveState(state) // best-effort
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
		deleteFiles := c.Bool("delete-files")
		ids := c.Args().Slice()
		if len(ids) == 0 {
			return fmt.Errorf("must specify at least one task id")
		}

		p, err := poolFromContext(ctx, c)
		if err != nil {
			return err
		}

		// Single-account mode.
		if p == nil {
			client, acc, err := clientFromContext(ctx, c)
			if err != nil {
				return err
			}
			if err := client.DeleteTasks(ctx, deleteFiles, ids); err != nil {
				return err
			}
			fmt.Printf("account: %s delete tasks deleteFiles:%v OK\n", acc.Alias, deleteFiles)
			// Clean up local state.
			state, _ := pool.LoadState()
			if state != nil {
				for _, id := range ids {
					state.RemoveTask(acc.Alias, id)
				}
				pool.SaveState(state)
			}
			return nil
		}

		// Multi-account mode: find the owning account for each task ID.
		state, err := pool.LoadState()
		if err != nil {
			return err
		}
		byAccount := make(map[string][]string) // alias -> task IDs
		var unknown []string
		for _, id := range ids {
			owner := state.FindTaskOwner(id)
			if owner == "" {
				unknown = append(unknown, id)
			} else {
				byAccount[owner] = append(byAccount[owner], id)
			}
		}
		if len(unknown) > 0 {
			fmt.Printf("unknown task ids (not in local state): %s\n", strings.Join(unknown, ", "))
		}

		for alias, taskIDs := range byAccount {
			client, acc, err := p.ClientForAlias(ctx, alias)
			if err != nil {
				fmt.Printf("account=%s error=%v\n", alias, err)
				continue
			}
			if err := client.DeleteTasks(ctx, deleteFiles, taskIDs); err != nil {
				fmt.Printf("account=%s error=%v\n", alias, err)
				continue
			}
			fmt.Printf("account: %s delete tasks deleteFiles:%v OK\n", acc.Alias, deleteFiles)
			for _, id := range taskIDs {
				state.RemoveTask(alias, id)
			}
		}
		return pool.SaveState(state)
	},
}

var taskClearCmd = &cli.Command{
	Name:  "clear",
	Usage: "clear offline tasks",
	Flags: []cli.Flag{
		deleteFilesFlag,
	},
	Action: func(ctx context.Context, c *cli.Command) error {
		deleteFiles := c.Bool("delete-files")

		p, err := poolFromContext(ctx, c)
		if err != nil {
			return err
		}

		// Single-account mode.
		if p == nil {
			client, acc, err := clientFromContext(ctx, c)
			if err != nil {
				return err
			}
			if err := client.ClearTasks(ctx, deleteFiles); err != nil {
				return err
			}
			fmt.Printf("account: %s clear tasks --delete-files:%v OK\n", acc.Alias, deleteFiles)
			state, _ := pool.LoadState()
			if state != nil {
				state.ClearTasks(acc.Alias)
				pool.SaveState(state)
			}
			return nil
		}

		// Multi-account mode: clear tasks for all accounts that have them.
		state, err := pool.LoadState()
		if err != nil {
			return err
		}
		aliases := state.AccountsWithTasks()
		if len(aliases) == 0 {
			fmt.Println("no tasks recorded in local state")
			return nil
		}

		for _, alias := range aliases {
			client, acc, err := p.ClientForAlias(ctx, alias)
			if err != nil {
				fmt.Printf("account=%s error=%v\n", alias, err)
				continue
			}
			if err := client.ClearTasks(ctx, deleteFiles); err != nil {
				fmt.Printf("account=%s error=%v\n", alias, err)
				continue
			}
			fmt.Printf("account: %s clear tasks --delete-files:%v OK\n", acc.Alias, deleteFiles)
			state.ClearTasks(alias)
		}
		return pool.SaveState(state)
	},
}
