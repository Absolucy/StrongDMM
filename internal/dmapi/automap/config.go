package automap

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"sdmm/internal/dmapi/dmmap"

	"github.com/BurntSushi/toml"
)

// Entry is one [templates.NAME] table of automapper_config.toml.
type Entry struct {
	Name        string   `toml:"-"`
	MapFiles    []string `toml:"map_files"`
	Directory   string   `toml:"directory"`
	RequiredMap string   `toml:"required_map"`
	Coordinates []int    `toml:"coordinates"`
	TraitName   string   `toml:"trait_name"`
}

// Config is the project's automapper_config.toml plus where new templates should go.
// Paths inside it (Directory, TemplateRoot) are repo-relative with forward slashes, same as the file uses.
type Config struct {
	RootDir string
	Path    string

	TemplateRoot  string
	templateRoots []string

	Entries []*Entry
}

var ValidName = regexp.MustCompile(`^[a-z0-9_]+$`)

// FindConfig loads the one _maps/*/automapper/automapper_config.toml under rootDir.
// No config at all means the project doesn't use the automapper, so that's (nil, nil).
func FindConfig(rootDir string) (*Config, error) {
	configs, _ := filepath.Glob(filepath.Join(rootDir, "_maps", "*", "automapper", "automapper_config.toml"))
	if len(configs) == 0 {
		return nil, nil
	}
	if len(configs) > 1 {
		return nil, fmt.Errorf("expected one automapper config, found %d: %s", len(configs), strings.Join(configs, ", "))
	}

	cfg := &Config{RootDir: rootDir, Path: configs[0]}
	if err := cfg.findTemplateRoots(); err != nil {
		return nil, err
	}
	if err := cfg.load(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// A templates folder with no config beside it is a downstream fork's own tree (Oculis keeps its
// templates in _maps/oculis but inherits Nova's config), so new templates go there if one exists.
func (c *Config) findTemplateRoots() error {
	roots, _ := filepath.Glob(filepath.Join(c.RootDir, "_maps", "*", "automapper", "templates"))
	var ownRoots []string
	for _, root := range roots {
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			continue
		}
		rel := c.relative(root)
		c.templateRoots = append(c.templateRoots, rel)
		if _, err := os.Stat(filepath.Join(filepath.Dir(root), "automapper_config.toml")); errors.Is(err, os.ErrNotExist) {
			ownRoots = append(ownRoots, rel)
		}
	}

	switch len(ownRoots) {
	case 0:
		c.TemplateRoot = c.relative(filepath.Join(filepath.Dir(c.Path), "templates"))
	case 1:
		c.TemplateRoot = ownRoots[0]
	default:
		return fmt.Errorf("more than one templates folder without a config beside it: %s", strings.Join(ownRoots, ", "))
	}
	return nil
}

func (c *Config) load() error {
	var parsed struct {
		Templates map[string]*Entry `toml:"templates"`
	}
	meta, err := toml.DecodeFile(c.Path, &parsed)
	if err != nil {
		return fmt.Errorf("parsing %s: %w", c.Path, err)
	}

	// the decoded map forgets file order, but the game loads templates in it
	c.Entries = nil
	for _, key := range meta.Keys() {
		if len(key) != 2 || key[0] != "templates" {
			continue
		}
		entry := parsed.Templates[key[1]]
		entry.Name = key[1]
		c.Entries = append(c.Entries, entry)
	}
	return nil
}

func (c *Config) Entry(name string) *Entry {
	for _, e := range c.Entries {
		if e.Name == name {
			return e
		}
	}
	return nil
}

// Monkestation's required_map is the path under _maps/, since some maps share a filename
// (Oshan station and trench are both Oshan.dmm). Older configs only have the filename.
func (e *Entry) wants(dmm *dmmap.Dmm) bool {
	if strings.Contains(e.RequiredMap, "/") {
		return strings.EqualFold(e.RequiredMap, mapsPath(dmm))
	}
	return strings.EqualFold(e.RequiredMap, dmm.Name)
}

// requiredMapOf is what a new entry for dmm should say, in whichever form the config already uses.
func (c *Config) requiredMapOf(dmm *dmmap.Dmm) string {
	p := mapsPath(dmm)
	if p == "" {
		return dmm.Name
	}
	for _, e := range c.Entries {
		if strings.Contains(e.RequiredMap, "/") {
			return p
		}
	}
	return dmm.Name
}

// mapsPath is dmm's path under _maps/, or "" if it lives somewhere else.
func mapsPath(dmm *dmmap.Dmm) string {
	rel, ok := strings.CutPrefix(filepath.ToSlash(dmm.Path.Readable), "_maps/")
	if !ok {
		return ""
	}
	return rel
}

// TemplatePath is where the entry's (first) map file lives on disk.
func (c *Config) TemplatePath(e *Entry) string {
	return filepath.Join(c.RootDir, filepath.FromSlash(e.Directory), e.MapFiles[0])
}

// NewTemplateDirectory picks the config "directory" for a new template of the given station map.
// It reuses an existing per-station folder so the casing matches whatever's already there.
func (c *Config) NewTemplateDirectory(mapStem string) string {
	for _, root := range c.templateRoots {
		entries, _ := os.ReadDir(filepath.Join(c.RootDir, filepath.FromSlash(root)))
		for _, entry := range entries {
			if entry.IsDir() && strings.EqualFold(entry.Name(), mapStem) {
				return path.Join(c.TemplateRoot, entry.Name()) + "/"
			}
		}
	}
	return path.Join(c.TemplateRoot, strings.ToLower(mapStem)) + "/"
}

func (c *Config) relative(p string) string {
	rel, err := filepath.Rel(c.RootDir, p)
	if err != nil {
		return filepath.ToSlash(p)
	}
	return filepath.ToSlash(rel)
}

// The edits below work on the raw text so comments and layout survive, same as automapper-gen does it.

func (c *Config) Append(e *Entry) error {
	return c.edit(func(text string) (string, error) {
		return appendEntry(text, e), nil
	})
}

func (c *Config) SetCoordinates(name string, coordinates [3]int) error {
	return c.edit(func(text string) (string, error) {
		return setCoordinates(text, name, coordinates)
	})
}

func (c *Config) Remove(name string) error {
	return c.edit(func(text string) (string, error) {
		return removeEntry(text, name)
	})
}

func (c *Config) edit(change func(string) (string, error)) error {
	raw, err := os.ReadFile(c.Path)
	if err != nil {
		return fmt.Errorf("reading %s: %w", c.Path, err)
	}
	// autocrlf checkouts are CRLF, and the edits (plus the regex's $) only speak \n
	crlf := strings.Contains(string(raw), "\r\n")
	text, err := change(strings.ReplaceAll(string(raw), "\r\n", "\n"))
	if err != nil {
		return err
	}
	if crlf {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if err = os.WriteFile(c.Path, []byte(text), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", c.Path, err)
	}
	return c.load()
}

func appendEntry(text string, e *Entry) string {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return text + fmt.Sprintf(
		"\n# %s\n[templates.%s]\nmap_files = [\"%s\"]\ndirectory = \"%s\"\nrequired_map = \"%s\"\ncoordinates = [%d, %d, %d]\ntrait_name = \"%s\"\n",
		e.Name, e.Name, e.MapFiles[0], e.Directory, e.RequiredMap,
		e.Coordinates[0], e.Coordinates[1], e.Coordinates[2], e.TraitName,
	)
}

var coordinatesLine = regexp.MustCompile(`(?m)^coordinates = \[.*\]$`)

func setCoordinates(text, name string, coordinates [3]int) (string, error) {
	start, end, err := sectionBounds(text, name)
	if err != nil {
		return "", err
	}
	section := text[start:end]
	loc := coordinatesLine.FindStringIndex(section)
	if loc == nil {
		return "", fmt.Errorf("no coordinates line under [templates.%s]", name)
	}
	line := fmt.Sprintf("coordinates = [%d, %d, %d]", coordinates[0], coordinates[1], coordinates[2])
	return text[:start] + section[:loc[0]] + line + section[loc[1]:] + text[end:], nil
}

// removeEntry drops the table plus the comment line right above its header, which is how
// every entry in these configs is laid out. Comments that lead into the next table stay put.
func removeEntry(text, name string) (string, error) {
	start, end, err := sectionBounds(text, name)
	if err != nil {
		return "", err
	}

	lines := strings.SplitAfter(text[start:end], "\n")
	for len(lines) > 1 {
		last := strings.TrimSpace(lines[len(lines)-1])
		if last != "" && !strings.HasPrefix(last, "#") {
			break
		}
		lines = lines[:len(lines)-1]
	}
	end = start + len(strings.Join(lines, ""))

	before := text[:start]
	if idx := strings.LastIndex(strings.TrimSuffix(before, "\n"), "\n"); idx != -1 {
		if strings.HasPrefix(strings.TrimSpace(before[idx+1:]), "#") {
			before = before[:idx+1]
		}
	}
	before = strings.TrimRight(before, "\n")
	after := strings.TrimLeft(text[end:], "\n")
	if before == "" {
		return after, nil
	}
	if after == "" {
		return before + "\n", nil
	}
	return before + "\n\n" + after, nil
}

// sectionBounds finds [templates.NAME] up to the next table header.
func sectionBounds(text, name string) (int, int, error) {
	header := "[templates." + name + "]"
	start := -1
	for offset := 0; ; {
		idx := strings.Index(text[offset:], header)
		if idx == -1 {
			break
		}
		idx += offset
		if idx == 0 || text[idx-1] == '\n' {
			start = idx
			break
		}
		offset = idx + len(header)
	}
	if start == -1 {
		return 0, 0, fmt.Errorf("no [templates.%s] in the automapper config", name)
	}

	end := len(text)
	if next := strings.Index(text[start+len(header):], "\n["); next != -1 {
		end = start + len(header) + next + 1
	}
	return start, end, nil
}
