// smoke is a manual smoke test that drives the vshell client against a live
// v_shell instance. Run with VSHELL_URL / VSHELL_USERNAME / VSHELL_PASSWORD set.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/chinaran404/vshell-mcp/internal/vshell"
)

func must(err error) {
	if err != nil {
		fmt.Println("FAIL:", err)
		os.Exit(1)
	}
}

func main() {
	cfg := vshell.Config{
		BaseURL:  os.Getenv("VSHELL_URL"),
		Username: os.Getenv("VSHELL_USERNAME"),
		Password: os.Getenv("VSHELL_PASSWORD"),
		Timeout:  90 * time.Second,
	}
	c := vshell.NewClient(cfg)
	ctx := context.Background()

	fmt.Println("== list hosts ==")
	hosts, err := c.ListHosts(ctx, nil)
	must(err)
	for _, h := range hosts {
		fmt.Printf("  id=%d %s host=%s os=%s user=%s online=%v\n", h.Id, h.Addr, h.HostName, h.OsName, h.UserName, h.IsConnect)
	}
	if len(hosts) == 0 {
		fmt.Println("  (no hosts)")
		return
	}
	id := hosts[0].Id

	fmt.Println("== get disks ==")
	disks, err := c.GetDisks(ctx, id)
	must(err)
	for _, d := range disks {
		fmt.Printf("  %s mode=%s\n", d.Name, d.Mode)
	}

	fmt.Println("== list directory /tmp ==")
	entries, err := c.ListDirectory(ctx, id, "/tmp")
	must(err)
	for _, e := range entries[:min(5, len(entries))] {
		fmt.Printf("  %s dir=%v size=%d mode=%s\n", e.Name, e.IsDir, e.Size, e.Mode)
	}

	fmt.Println("== exec: id ==")
	out, err := c.Exec(ctx, id, "id", "", 60*time.Second)
	must(err)
	fmt.Printf("  -> %q\n", out)

	fmt.Println("== exec: uname -a (workdir /var/log) ==")
	out, err = c.Exec(ctx, id, "uname -a && pwd", "/var/log", 60*time.Second)
	must(err)
	fmt.Printf("  -> %q\n", out)

	fmt.Println("== write file ==")
	must(c.WriteFile(ctx, id, "/tmp", "mcp_smoke.txt", "hello from vshell-mcp smoke\nline2\n"))

	fmt.Println("== read file ==")
	content, b64, err := c.ReadFile(ctx, id, "/tmp", "mcp_smoke.txt")
	must(err)
	fmt.Printf("  -> base64=%v content=%q\n", b64, content)

	fmt.Println("== upload file ==")
	tmp, _ := os.CreateTemp("", "mcp_smoke_up_*.txt")
	fmt.Fprintf(tmp, "uploaded content %d\n", time.Now().Unix())
	tmp.Close()
	defer os.Remove(tmp.Name())
	remote, err := c.UploadFile(ctx, id, "/tmp", tmp.Name(), "mcp_smoke_upload.txt")
	must(err)
	fmt.Printf("  -> remote=%s\n", remote)

	fmt.Println("== download file ==")
	dl, _ := os.CreateTemp("", "mcp_smoke_dl_*.txt")
	dl.Close()
	os.Remove(dl.Name())
	n, err := c.DownloadFile(ctx, id, "/tmp", "mcp_smoke.txt", dl.Name())
	must(err)
	data, _ := os.ReadFile(dl.Name())
	fmt.Printf("  -> bytes=%d content=%q\n", n, string(data))

	fmt.Println("== rename ==")
	must(c.RenameFile(ctx, id, "/tmp", "mcp_smoke.txt", "mcp_smoke_renamed.txt"))

	fmt.Println("== create dir + file ==")
	must(c.CreateDirectory(ctx, id, "/tmp", "mcp_smoke_dir"))
	must(c.CreateFile(ctx, id, "/tmp/mcp_smoke_dir", "empty.txt"))

	fmt.Println("== delete ==")
	must(c.DeleteFile(ctx, id, "/tmp", "mcp_smoke_renamed.txt"))
	must(c.DeleteFile(ctx, id, "/tmp", "mcp_smoke_upload.txt"))
	must(c.DeleteFiles(ctx, id, "/tmp", []string{"mcp_smoke_dir"}))

	fmt.Println("ALL SMOKE TESTS PASSED")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
