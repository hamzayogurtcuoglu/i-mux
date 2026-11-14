package theme

import "fmt"

// Colors are the colors a config overrides ("" keeps the theme's).
type Colors struct {
	Accent      string `toml:"accent"`
	PanelBg     string `toml:"panel_bg"`
	SidebarBg   string `toml:"sidebar_bg"`
	ActiveRowBg string `toml:"active_row_bg"`
	SelectionBg string `toml:"selection_bg"`
	Surface0    string `toml:"surface0"`
	Surface1    string `toml:"surface1"`
	SurfaceDim  string `toml:"surface_dim"`
	Overlay0    string `toml:"overlay0"`
	Overlay1    string `toml:"overlay1"`
	Text        string `toml:"text"`
	Subtext0    string `toml:"subtext0"`
	Mauve       string `toml:"mauve"`
	Green       string `toml:"green"`
	Yellow      string `toml:"yellow"`
	Red         string `toml:"red"`
	Blue        string `toml:"blue"`
	Teal        string `toml:"teal"`
	Peach       string `toml:"peach"`
}

// values are c's strings in Tokens' order.
func (c *Colors) values() []*string {
	return []*string{
		&c.Accent, &c.PanelBg, &c.SidebarBg, &c.ActiveRowBg, &c.SelectionBg, &c.Surface0, &c.Surface1,
		&c.SurfaceDim, &c.Overlay0, &c.Overlay1, &c.Text, &c.Subtext0, &c.Mauve, &c.Green, &c.Yellow, &c.Red,
		&c.Blue, &c.Teal, &c.Peach,
	}
}

// Custom is [theme.custom]: colors for every mode, then for light or dark
// only ([theme.custom.light], [theme.custom.dark]).
type Custom struct {
	Colors
	Light Colors `toml:"light"`
	Dark  Colors `toml:"dark"`
}

// Config is the [theme] section.
type Config struct {
	Name       string `toml:"name"`
	AutoSwitch bool   `toml:"auto_switch"`
	LightName  string `toml:"light_name"` // "" = name's light sibling
	DarkName   string `toml:"dark_name"`  // "" = name's dark sibling
	Custom     Custom `toml:"custom"`
}

// Check reports the config's first bad color, by its key
// ("theme.custom.light.accent"), and warns of theme names that are no
// theme (they are used as the default).
func (c Config) Check() (warnings []string, key string, err error) {
	for _, n := range []struct{ key, val string }{{"name", c.Name}, {"light_name", c.LightName}, {"dark_name", c.DarkName}} {
		if n.val == "" {
			continue
		}
		if _, ok := Canonical(n.val); !ok {
			fallback := Default
			if n.key == "light_name" {
				fallback = LightOf(Default)
			}
			warnings = append(warnings, fmt.Sprintf("unknown theme name theme.%s = %q; using %q", n.key, n.val, fallback))
		}
	}
	for _, sec := range []struct {
		prefix string
		cs     Colors
	}{{"theme.custom.", c.Custom.Colors}, {"theme.custom.light.", c.Custom.Light}, {"theme.custom.dark.", c.Custom.Dark}} {
		for i, v := range sec.cs.values() {
			if *v == "" {
				continue
			}
			if _, err := ParseColor(*v); err != nil {
				return warnings, sec.prefix + Tokens[i], err
			}
		}
	}
	return warnings, "", nil
}

// themeName is a configured name, or fallback when it is empty or no theme.
func themeName(name, fallback string) string {
	if n, ok := Canonical(name); ok {
		return n
	}
	return fallback
}

// Effective is the theme in use for appearance ("light", "dark", or ""
// unknown, which is dark): name; with auto_switch, light_name or
// dark_name (name's sibling when unset).
func (c Config) Effective(appearance string) string {
	name := themeName(c.Name, Default)
	if !c.AutoSwitch {
		return name
	}
	if appearance == AppearanceLight {
		return themeName(c.LightName, LightOf(name))
	}
	return themeName(c.DarkName, DarkOf(name))
}

// Resolve is the palette for appearance: the effective theme, then
// [theme.custom], then the mode's colors: the terminal's light or dark when
// the theme follows it (dark when not known), else the theme's own.
func Resolve(c Config, appearance string) Palette {
	name := c.Effective(appearance)
	p := Builtin(name)
	light := IsLight(name)
	if c.AutoSwitch {
		light = appearance == AppearanceLight
	}
	mode := c.Custom.Dark
	if light {
		mode = c.Custom.Light
	}
	for _, cs := range []Colors{c.Custom.Colors, mode} {
		for i, v := range cs.values() {
			if *v == "" {
				continue
			}
			if col, err := ParseColor(*v); err == nil {
				*p.colors()[i] = col
			}
		}
	}
	return p
}
