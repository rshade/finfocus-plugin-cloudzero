package main

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/plugin"
)

func TestBinaryAnnouncesPortAndCapabilities(t *testing.T) {
	t.Parallel()
	bin := filepath.Join(t.TempDir(), "finfocus-plugin-cloudzero")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	out, err := build.CombinedOutput()
	require.NoError(t, err, string(out))

	cmd := exec.Command(bin)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})

	line, err := bufio.NewReader(stdout).ReadString('\n')
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(line, "PORT="), line)
	port, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "PORT=")))
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.NewClient(
		"127.0.0.1:"+strconv.Itoa(port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	info, err := pbc.NewCostSourceServiceClient(conn).GetPluginInfo(ctx, &pbc.GetPluginInfoRequest{})
	require.NoError(t, err)
	require.Equal(t, plugin.Info(version).Capabilities, info.GetCapabilities())
	require.Contains(t, info.GetCapabilities(), pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS)
	require.Contains(t, info.GetCapabilities(), pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS)
}
