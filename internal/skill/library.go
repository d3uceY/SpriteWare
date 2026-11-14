package skill

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// The skill ships in three tiers, and this file turns them into one searchable
// library so a tool can route a task instead of guessing at a URL:
//
//	SKILL.md            the entry document
//	<workflow>/SKILL.md one procedure, its own rules and triggers
//	rules/*.md          the subject chapters, cited as rules://<id>
//	references/*.md     the lookup notes, published as rules://<name>
//
// Routing is lexical on purpose: the corpus is embedded and small, and a
// deterministic scorer with a visible matched-token list can be tested and
// argued with, which an opaque ranking cannot.

// Workflow is one procedure: a folder holding a SKILL.md.
type Workflow struct {
	Name        string
	Description string
	// Path is the embed path, e.g. "draw/SKILL.md".
	Path string
	// Body is the markdown after the frontmatter block.
	Body string
	// Rules are the chapter IDs the workflow claims as its own, from the
	// `rules:` frontmatter line. They come first in a guide.
	Rules []string
	// Triggers are extra routing words from the `triggers:` frontmatter line.
	// They are how a task that never uses the workflow's own words still
	// reaches it ("godot" for export).
	Triggers []string
	// References are the lookup notes the body links.
	References []string
}

// Chapter is one subject chapter from rules/.
type Chapter struct {
	// ID is the file stem, e.g. "34-hands-and-feet".
	ID    string
	Title string
	// Band groups the chapters the way the index does: core, technique,
	// colour, character, animation, creature, environment, object, effects,
	// style.
	Band string
	Path string
	Body string
}

// Reference is one lookup note from references/.
type Reference struct {
	Name  string
	Title string
	Path  string
	Body  string
}

// Route is one ranked workflow match for a task.
type Route struct {
	Workflow *Workflow
	// Score is the accumulated IDF-weighted token overlap. It is only
	// comparable within one call.
	Score float64
	// Matched lists the task tokens that contributed, with "(named)" and
	// "(trigger)" marking the words that make a match confident.
	Matched []string
}

// Hit is one ranked chapter match for a search query.
type Hit struct {
	Chapter *Chapter
	Score   float64
	Why     string
}

// Library is the parsed, immutable skill tree.
type Library struct {
	// Entry is the root SKILL.md: what the skill is and how to route.
	Entry *Workflow

	workflows     map[string]*Workflow
	workflowOrder []string
	chapters      map[string]*Chapter
	chapterOrder  []string
	refs          map[string]*Reference
	refOrder      []string

	// index is rules/README.md, served as rules://index. It is not a chapter.
	index string

	// docFreq counts how many workflows carry a token, for IDF weighting.
	docFreq map[string]int
}

var (
	loadOnce sync.Once
	loadLib  *Library
	loadErr  error
)

// Default returns the embedded library, parsing it on first call. The embedded
// filesystem cannot change at runtime, so one parse is enough.
func Default() (*Library, error) {
	loadOnce.Do(func() {
		loadLib, loadErr = Load()
	})
	return loadLib, loadErr
}

// Load parses every markdown file in the skill. It returns an error rather than
// a partial library when a file is malformed, so a broken build fails a test
// instead of silently routing an agent to the wrong chapter.
func Load() (*Library, error) {
	lib := &Library{
		workflows: make(map[string]*Workflow),
		chapters:  make(map[string]*Chapter),
		refs:      make(map[string]*Reference),
		docFreq:   make(map[string]int),
	}

	for _, rel := range Files() {
		body, err := Read(rel)
		if err != nil {
			return nil, fmt.Errorf("skill: read %s: %w", rel, err)
		}

		switch {
		case rel == "SKILL.md":
			entry, err := parseWorkflow(Name, rel, body)
			if err != nil {
				return nil, err
			}
			lib.Entry = entry

		case strings.HasPrefix(rel, "rules/"):
			if rel == "rules/README.md" {
				lib.index = body
				continue
			}
			chapter := parseChapter(rel, body)
			if _, duplicate := lib.chapters[chapter.ID]; duplicate {
				return nil, fmt.Errorf("skill: duplicate chapter %q", chapter.ID)
			}
			lib.chapters[chapter.ID] = chapter
			lib.chapterOrder = append(lib.chapterOrder, chapter.ID)

		case strings.HasPrefix(rel, "references/"):
			name := strings.TrimSuffix(strings.TrimPrefix(rel, "references/"), ".md")
			lib.refs[name] = &Reference{Name: name, Title: firstHeading(body), Path: rel, Body: body}
			lib.refOrder = append(lib.refOrder, name)

		case strings.HasSuffix(rel, "/SKILL.md"):
			folder := path.Dir(rel)
			workflow, err := parseWorkflow(folder, rel, body)
			if err != nil {
				return nil, err
			}
			if _, duplicate := lib.workflows[workflow.Name]; duplicate {
				return nil, fmt.Errorf("skill: duplicate workflow %q", workflow.Name)
			}
			lib.workflows[workflow.Name] = workflow
			lib.workflowOrder = append(lib.workflowOrder, workflow.Name)
		}
	}

	sort.Strings(lib.workflowOrder)
	sort.Strings(lib.chapterOrder)
	sort.Strings(lib.refOrder)
	lib.indexWorkflows()
	return lib, nil
}

// indexWorkflows builds the token document frequencies routing needs.
func (l *Library) indexWorkflows() {
	for _, name := range l.workflowOrder {
		seen := make(map[string]bool)
		for _, token := range tokenize(l.haystack(l.workflows[name])) {
			seen[token] = true
		}
		for token := range seen {
			l.docFreq[token]++
		}
	}
}

// haystack is every word a workflow can be routed by.
func (l *Library) haystack(w *Workflow) string {
	return w.Name + " " + w.Description + " " + strings.Join(w.Triggers, " ")
}

// WorkflowNames lists the workflows in alphabetical order.
func (l *Library) WorkflowNames() []string {
	return append([]string(nil), l.workflowOrder...)
}

// Workflow resolves a workflow by name.
func (l *Library) Workflow(name string) (*Workflow, bool) {
	w, ok := l.workflows[strings.ToLower(strings.TrimSpace(name))]
	return w, ok
}

// ChapterIDs lists the subject chapters in alphabetical order.
func (l *Library) ChapterIDs() []string {
	return append([]string(nil), l.chapterOrder...)
}

// Chapter resolves a chapter by exact ID, or by its bare number ("34").
func (l *Library) Chapter(id string) (*Chapter, bool) {
	query := strings.ToLower(strings.TrimSpace(id))
	query = strings.TrimSuffix(query, ".md")
	query = strings.TrimPrefix(query, "rules://")
	query = strings.TrimPrefix(query, "rules/")
	if c, ok := l.chapters[query]; ok {
		return c, true
	}
	if number := digitsRe.FindString(query); number != "" {
		for _, candidate := range l.chapterOrder {
			if strings.HasPrefix(candidate, number+"-") {
				return l.chapters[candidate], true
			}
		}
	}
	return nil, false
}

// ReferenceNames lists the lookup notes in alphabetical order.
func (l *Library) ReferenceNames() []string {
	return append([]string(nil), l.refOrder...)
}

// Reference resolves a lookup note by file stem.
func (l *Library) Reference(name string) (*Reference, bool) {
	r, ok := l.refs[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(name, "rules://")))]
	return r, ok
}

// RuleIndexBody returns rules/README.md, served as rules://index.
func (l *Library) RuleIndexBody() (string, error) {
	if l.index == "" {
		return "", fmt.Errorf("skill: rules/README.md is not embedded")
	}
	return l.index, nil
}

// Route ranks workflows for a free-text task. Scoring is IDF-weighted token
// overlap over the name, the description and the triggers, so a rare word
// counts far more than "sprite", which nearly every description contains.
func (l *Library) Route(task string, limit int) []Route {
	tokens := uniqueTokens(task)

	type scored struct {
		name    string
		score   float64
		matched []string
	}
	all := make([]scored, 0, len(l.workflowOrder))

	for _, name := range l.workflowOrder {
		w := l.workflows[name]

		present := make(map[string]bool)
		for _, token := range tokenize(l.haystack(w)) {
			present[token] = true
		}
		nameTokens := make(map[string]bool)
		for _, token := range tokenize(w.Name) {
			nameTokens[token] = true
		}
		triggerTokens := make(map[string]bool)
		for _, trigger := range w.Triggers {
			for _, token := range tokenize(trigger) {
				triggerTokens[token] = true
			}
		}

		var score float64
		var matched []string
		for _, token := range tokens {
			if !present[token] {
				continue
			}
			weight := math.Log(float64(len(l.workflowOrder)+1) / float64(l.docFreq[token]))
			if weight <= 0 {
				weight = 0.01
			}
			switch {
			case nameTokens[token]:
				weight *= 2
				matched = append(matched, token)
			case triggerTokens[token]:
				weight *= 2
				matched = append(matched, token+" (trigger)")
			default:
				matched = append(matched, token)
			}
			score += weight
		}

		// A task that names the deliverable outright ("export this", "an
		// animation") should beat a workflow that merely mentions the words.
		for _, token := range tokens {
			if nameTokens[token] {
				score += 5
				matched = append(matched, token+" (named)")
				break
			}
		}

		all = append(all, scored{name: name, score: score, matched: matched})
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].name < all[j].name
	})

	if limit <= 0 || limit > len(all) {
		limit = len(all)
	}
	routes := make([]Route, 0, limit)
	for _, entry := range all[:limit] {
		routes = append(routes, Route{Workflow: l.workflows[entry.name], Score: entry.score, Matched: entry.matched})
	}
	return routes
}

// Search ranks subject chapters for a query, using the same IDF weighting over
// titles, IDs and bodies. It answers "which chapter covers this subject"; Route
// answers "which procedure applies".
func (l *Library) Search(query string, limit int) []Hit {
	tokens := uniqueTokens(query)
	if len(tokens) == 0 {
		return nil
	}

	df := make(map[string]int, len(tokens))
	for _, id := range l.chapterOrder {
		body := strings.ToLower(l.chapters[id].Title + " " + l.chapters[id].ID + " " + l.chapters[id].Body)
		for _, token := range tokens {
			if strings.Contains(body, token) {
				df[token]++
			}
		}
	}

	var hits []Hit
	for _, id := range l.chapterOrder {
		chapter := l.chapters[id]
		title := strings.ToLower(chapter.Title)
		body := strings.ToLower(chapter.Title + " " + chapter.ID + " " + chapter.Body)
		var score float64
		var why []string
		for _, token := range tokens {
			if !strings.Contains(body, token) {
				continue
			}
			weight := math.Log(float64(len(l.chapterOrder)+1) / float64(df[token]+1))
			if weight <= 0 {
				weight = 0.01
			}
			if strings.Contains(title, token) {
				weight *= 3
				why = append(why, token+" (title)")
			} else {
				why = append(why, token)
			}
			score += weight
		}
		if score > 0 {
			hits = append(hits, Hit{Chapter: chapter, Score: score, Why: strings.Join(why, ", ")})
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Chapter.ID < hits[j].Chapter.ID
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// Validate checks the library's structural contract: names match folders,
// every workflow names chapters that exist and carries usable triggers, and
// every chapter has a title and a known band. It is exported so a test can
// point at the failure instead of a downstream nil.
func (l *Library) Validate() error {
	if l.Entry == nil {
		return fmt.Errorf("skill: missing root SKILL.md")
	}
	if l.Entry.Name == "" || l.Entry.Description == "" {
		return fmt.Errorf("skill: SKILL.md needs name and description frontmatter")
	}
	if len(l.workflows) == 0 {
		return fmt.Errorf("skill: no workflows found")
	}

	for _, name := range l.workflowOrder {
		w := l.workflows[name]
		if w.Name != path.Base(path.Dir(w.Path)) {
			return fmt.Errorf("skill: %s declares name %q, folder is %q", w.Path, w.Name, path.Base(path.Dir(w.Path)))
		}
		if n := len([]rune(w.Description)); n < 1 || n > 1024 {
			return fmt.Errorf("skill: %s description is %d characters, the limit is 1-1024", w.Path, n)
		}
		if len(w.Rules) < 3 {
			return fmt.Errorf("skill: %s names %d rules; name at least 3", w.Path, len(w.Rules))
		}
		for _, id := range w.Rules {
			if _, ok := l.Chapter(id); !ok {
				return fmt.Errorf("skill: %s names missing chapter %q", w.Path, id)
			}
		}
		if len(w.Triggers) < 3 {
			return fmt.Errorf("skill: %s has %d triggers; list at least 3", w.Path, len(w.Triggers))
		}
		seen := make(map[string]bool)
		for _, trigger := range w.Triggers {
			if trigger != strings.ToLower(strings.TrimSpace(trigger)) || trigger == "" {
				return fmt.Errorf("skill: %s trigger %q is not lowercase", w.Path, trigger)
			}
			if seen[trigger] {
				return fmt.Errorf("skill: %s repeats trigger %q", w.Path, trigger)
			}
			if len(tokenize(trigger)) == 0 {
				return fmt.Errorf("skill: %s trigger %q carries no indexable word", w.Path, trigger)
			}
			seen[trigger] = true
		}
		for _, ref := range w.References {
			if _, ok := l.Reference(ref); !ok {
				return fmt.Errorf("skill: %s links missing reference %q", w.Path, ref)
			}
		}
	}

	for _, id := range l.chapterOrder {
		chapter := l.chapters[id]
		if chapter.Title == "" {
			return fmt.Errorf("skill: rules/%s.md has no H1 title", id)
		}
		if chapter.Band == "" {
			return fmt.Errorf("skill: rules/%s.md is outside every band", id)
		}
	}
	for _, name := range l.refOrder {
		if l.refs[name].Title == "" {
			return fmt.Errorf("skill: references/%s.md has no H1 title", name)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Parsing
// ---------------------------------------------------------------------------

var (
	linkRe   = regexp.MustCompile(`\[[^\]]*\]\(([^)]+)\)`)
	digitsRe = regexp.MustCompile(`^\d+`)
)

// parseWorkflow reads one workflow's frontmatter and body.
func parseWorkflow(folder, rel, raw string) (*Workflow, error) {
	scalars, lists, body, err := parseFrontmatter(raw)
	if err != nil {
		return nil, fmt.Errorf("skill: %s: %w", rel, err)
	}
	w := &Workflow{
		Name:        scalars["name"],
		Description: scalars["description"],
		Path:        rel,
		Body:        body,
		Rules:       lists["rules"],
		Triggers:    lists["triggers"],
		References:  referenceLinks(body),
	}
	if w.Name == "" {
		w.Name = folder
	}
	return w, nil
}

// parseChapter reads one subject chapter. Chapters are copied verbatim from the
// aseprite-ai-artist project, so the only structure here is the H1 and the
// number the file starts with.
func parseChapter(rel, body string) *Chapter {
	id := strings.TrimSuffix(strings.TrimPrefix(rel, "rules/"), ".md")
	return &Chapter{ID: id, Title: firstHeading(body), Band: bandOf(id), Path: rel, Body: body}
}

// bandOf maps a chapter's number to the band the index groups it under.
func bandOf(id string) string {
	if id == "" {
		return ""
	}
	switch id[0] {
	case '0':
		return "core"
	case '1':
		return "technique"
	case '2':
		return "colour"
	case '3':
		return "character"
	case '4':
		return "animation"
	case '5':
		return "creature"
	case '6':
		return "environment"
	case '7':
		return "object"
	case '8':
		return "effects"
	case '9':
		return "style"
	}
	return ""
}

// parseFrontmatter splits a block of `key: value` and `key: [a, b]` lines from
// the markdown body. Only the scalar and flow-list forms the skill uses are
// supported, which keeps the parser dependency-free.
func parseFrontmatter(raw string) (map[string]string, map[string][]string, string, error) {
	scalars := make(map[string]string)
	lists := make(map[string][]string)

	text := strings.ReplaceAll(raw, "\r\n", "\n")
	if !strings.HasPrefix(text, "---\n") {
		// No frontmatter is legal for a chapter; a workflow without it would
		// fall back to its folder name and an empty description.
		return scalars, lists, text, nil
	}
	rest := text[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return nil, nil, "", fmt.Errorf("frontmatter is never closed")
	}
	body := strings.TrimPrefix(rest[end:], "\n---")
	body = strings.TrimPrefix(body, "\n")

	lastKey := ""
	for _, line := range strings.Split(rest[:end], "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			// A wrapped scalar continues on an indented line.
			if lastKey != "" {
				scalars[lastKey] += " " + trimmed
			}
			continue
		}
		key, value, found := strings.Cut(line, ":")
		if !found {
			return nil, nil, "", fmt.Errorf("bad frontmatter line %q", line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		lastKey = key
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			lists[key] = splitList(value)
			continue
		}
		scalars[key] = value
	}
	return scalars, lists, body, nil
}

// splitList reads the `[a, b, c]` flow-list form.
func splitList(value string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	out := make([]string, 0, 4)
	for _, item := range strings.Split(inner, ",") {
		item = strings.TrimSpace(strings.Trim(item, `"'`))
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

// referenceLinks collects the `references/*.md` links a workflow body makes.
func referenceLinks(body string) []string {
	var out []string
	seen := make(map[string]bool)
	for _, match := range linkRe.FindAllStringSubmatch(body, -1) {
		target := match[1]
		if !strings.Contains(target, "references/") {
			continue
		}
		name := target[strings.LastIndex(target, "/")+1:]
		name = strings.TrimSuffix(name, ".md")
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out
}

// firstHeading returns the first H1 of a document.
func firstHeading(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "# "))
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Tokens
// ---------------------------------------------------------------------------

// normalize lowercases and flattens punctuation so "hands-and-feet",
// "Hands and feet" and "hands_and_feet" compare equal.
func normalize(s string) string {
	s = strings.ToLower(s)
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}), " ")
}

// tokenize splits text into lowercase words of 3+ characters, dropping a
// trailing plural "s" so "tiles" matches "tile".
func tokenize(s string) []string {
	fields := strings.Fields(normalize(s))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if len(field) < 3 || stopWords[field] {
			continue
		}
		if len(field) > 4 && strings.HasSuffix(field, "s") {
			field = strings.TrimSuffix(field, "s")
		}
		out = append(out, field)
	}
	return out
}

// uniqueTokens is tokenize without repeats.
func uniqueTokens(s string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, token := range tokenize(s) {
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "that": true,
	"this": true, "from": true, "into": true, "your": true, "you": true,
	"our": true, "their": true, "them": true, "then": true, "than": true,
	"when": true, "what": true, "where": true, "which": true, "while": true,
	"have": true, "has": true, "will": true, "would": true, "should": true,
	"can": true, "make": true, "need": true, "want": true, "using": true,
	"use": true, "about": true, "just": true, "like": true, "some": true,
	"not": true, "are": true, "but": true, "all": true, "any": true,
	"how": true, "who": true, "its": true, "it": true, "is": true,
}
