// package manifest handles the dotfile registry data
// it loads creator info, categories, and dotfile metadata from json
package manifest

// Manifest represents the entire dotfile registry
// contains all creators and categories available
type Manifest struct {
	Version    string     `json:"version"`
	Categories []Category `json:"categories"`
	Creators   []Creator  `json:"creators"`
}

// Category groups creators by type
// like "tiling-wm" or "vim-wizards"
type Category struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Creator represents a content creator with their dotfiles
// each creator has a git repo and list of available configs
type Creator struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	GitHub      string    `json:"github"`
	Repo        string    `json:"repo"`
	Categories  []string  `json:"categories"`
	Description string    `json:"description"`
	Dotfiles    []Dotfile `json:"dotfiles"`
}

// Dotfile represents a single config file or set of files
// like tmux.conf or i3 config
type Dotfile struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Paths        []string `json:"paths"`
	Dependencies []string `json:"dependencies"`
}

// GetCategory finds a category by id
// returns nil if not found
func (m *Manifest) GetCategory(id string) *Category {
	for i := range m.Categories {
		if m.Categories[i].ID == id {
			return &m.Categories[i]
		}
	}
	return nil
}

// GetCreator finds a creator by id
// returns nil if not found
func (m *Manifest) GetCreator(id string) *Creator {
	for i := range m.Creators {
		if m.Creators[i].ID == id {
			return &m.Creators[i]
		}
	}
	return nil
}

// GetCreatorsByCategory filters creators by category id
// returns empty slice if none found
func (m *Manifest) GetCreatorsByCategory(categoryID string) []Creator {
	var result []Creator
	for _, creator := range m.Creators {
		for _, cat := range creator.Categories {
			if cat == categoryID {
				result = append(result, creator)
				break
			}
		}
	}
	return result
}

// GetDotfile finds a specific dotfile config from a creator
// returns nil if not found
func (c *Creator) GetDotfile(id string) *Dotfile {
	for i := range c.Dotfiles {
		if c.Dotfiles[i].ID == id {
			return &c.Dotfiles[i]
		}
	}
	return nil
}
