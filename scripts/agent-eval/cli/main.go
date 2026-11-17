// Command evalcli calls SpriteWare MCP tools over streamable HTTP and prints the
// result. Used by the agent rule-timing eval to set up a surface and read the
// output back from outside the agent under test.
//
//	evalcli --tools
//	evalcli guide @args.json
//	evalcli doctor '{}'
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func main() {
	listMode := len(os.Args) >= 2 && os.Args[1] == "--tools"
	if !listMode && len(os.Args) < 3 {
		fail("usage: evalcli <tool> <json-args|@file|->  |  evalcli --tools")
	}

	url := os.Getenv("SPRITEWARE_URL")
	if url == "" {
		url = "http://127.0.0.1:9933/mcp"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	c, err := client.NewStreamableHttpClient(url)
	if err != nil {
		fail("client: %v", err)
	}
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		fail("start: %v", err)
	}
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "evalcli", Version: "1"}
	if _, err := c.Initialize(ctx, init); err != nil {
		fail("initialize: %v", err)
	}
	if listMode {
		list(ctx, c)
		return
	}

	tool, argsJSON := os.Args[1], os.Args[2]
	var body []byte
	switch {
	case argsJSON == "-":
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			fail("stdin: %v", err)
		}
		body = raw
	case strings.HasPrefix(argsJSON, "@"):
		raw, err := os.ReadFile(strings.TrimPrefix(argsJSON, "@"))
		if err != nil {
			fail("args file: %v", err)
		}
		body = raw
	default:
		body = []byte(argsJSON)
	}

	var args map[string]any
	if err := json.Unmarshal(body, &args); err != nil {
		fail("args: %v", err)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = tool
	req.Params.Arguments = args

	res, err := c.CallTool(ctx, req)
	if err != nil {
		fail("call: %v", err)
	}
	for _, content := range res.Content {
		switch v := content.(type) {
		case mcp.TextContent:
			fmt.Println(v.Text)
		case *mcp.TextContent:
			fmt.Println(v.Text)
		}
	}
	if res.IsError {
		os.Exit(1)
	}
}

// list prints every tool with its op enum and required-field count, so a report
// can describe the surface the agent actually saw.
func list(ctx context.Context, c *client.Client) {
	res, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		fail("list tools: %v", err)
	}
	for _, tool := range res.Tools {
		props := tool.InputSchema.Properties
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		enum := ""
		if op, ok := props["op"].(map[string]any); ok {
			if values, ok := op["enum"].([]any); ok {
				parts := make([]string, 0, len(values))
				for _, v := range values {
					parts = append(parts, fmt.Sprint(v))
				}
				enum = strings.Join(parts, ",")
			}
		}
		fmt.Printf("%-10s req=%d enum=%s args=%s\n", tool.Name, len(tool.InputSchema.Required), enum, strings.Join(keys, " "))
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "evalcli: "+format+"\n", args...)
	os.Exit(1)
}
