package mcpserver

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/d3uceY/spriteware/internal/skill"
)

// The skill ships twice on purpose.
//
// An install writes a folder to disk; serving the same tree as MCP resources
// means a client that installs nothing still reads it on demand.
//
//	skill://spriteware      the entry document
//	skill://draw            one workflow
//	rules://craft           one reference chapter
const (
	skillScheme = "skill://"
	rulesScheme = "rules://"
)

// Resource is one published document.
type Resource struct {
	URI         string
	Name        string
	Description string
	Path        string
}

// Resources lists everything the server publishes, in a stable order.
func Resources() []Resource {
	out := make([]Resource, 0, 20)
	for _, rel := range skill.Files() {
		if !strings.HasSuffix(rel, ".md") {
			continue
		}
		out = append(out, resourceFor(rel))
	}
	// Generated documents: not skill files, but the same on-demand reading.
	out = append(out, Resource{
		URI:         "knowledge://palettes",
		Name:        "palettes",
		Description: "The bundled palette presets, with the colours and when to use each.",
		Path:        knowledgePath,
	})
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}

// knowledgePath marks a resource whose body is generated rather than embedded.
const knowledgePath = "generated:knowledge://palettes"

// resourceFor maps a file in the skill folder to a URI.
func resourceFor(rel string) Resource {
	body, _ := skill.Read(rel)

	switch {
	case rel == "SKILL.md":
		return Resource{URI: skillScheme + "spriteware", Name: "spriteware", Description: summary(body), Path: rel}

	case strings.HasPrefix(rel, "references/"):
		name := strings.TrimSuffix(strings.TrimPrefix(rel, "references/"), ".md")
		return Resource{URI: rulesScheme + name, Name: name, Description: summary(body), Path: rel}

	default:
		name := strings.TrimSuffix(rel, "/SKILL.md")
		return Resource{URI: skillScheme + name, Name: name, Description: summary(body), Path: rel}
	}
}

// summary is the frontmatter description, else the first paragraph. A resource
// with no description is one an agent will not think to read.
func summary(body string) string {
	text := strings.ReplaceAll(body, "\r\n", "\n")
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---"); end >= 0 {
			for _, line := range strings.Split(text[4:4+end], "\n") {
				if strings.HasPrefix(line, "description:") {
					return strings.TrimSpace(strings.TrimPrefix(line, "description:"))
				}
			}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return trimmed
		}
	}
	return ""
}

// ReadResource returns one published document by URI.
func ReadResource(uri string) (string, bool) {
	for _, r := range Resources() {
		if r.URI == uri {
			if r.Path == knowledgePath {
				return paletteDoc(), true
			}
			body, err := skill.Read(r.Path)
			return body, err == nil
		}
	}
	return "", false
}

// RegisterResources adds every document to the server.
func RegisterResources(srv *server.MCPServer) {
	for _, r := range Resources() {
		resource := mcp.NewResource(
			r.URI,
			r.Name,
			mcp.WithResourceDescription(r.Description),
			mcp.WithMIMEType("text/markdown"),
		)

		srv.AddResource(resource, func(ctx context.Context, req mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
			body, ok := ReadResource(req.Params.URI)
			if !ok {
				return nil, fmt.Errorf("no resource at %s", req.Params.URI)
			}
			return []mcp.ResourceContents{
				mcp.TextResourceContents{URI: req.Params.URI, MIMEType: "text/markdown", Text: body},
			}, nil
		})
	}
}

// ResourceIndex is appended to the server instructions so an agent knows the
// chapters exist.
func ResourceIndex() string {
	var sb strings.Builder
	sb.WriteString("\n\n## On-demand reading\n\nThese are MCP resources; read the one you need rather than guessing:\n")
	for _, r := range Resources() {
		sb.WriteString("- " + r.URI + " — " + r.Description + "\n")
	}
	return sb.String()
}
