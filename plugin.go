// Package plasmactldependencies implements a dependencies launchr plugin
package plasmactldependencies

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/launchrctl/launchr"
	"github.com/spf13/cobra"

	"github.com/skilld-labs/plasmactl-bump/v2/pkg/sync"
)

func init() {
	launchr.RegisterPlugin(&Plugin{})
}

// Plugin is [launchr.Plugin] providing dependencies search action.
type Plugin struct {
}

// PluginInfo implements [launchr.Plugin] interface.
func (p *Plugin) PluginInfo() launchr.PluginInfo {
	return launchr.PluginInfo{
		Weight: 20,
	}
}

// CobraAddCommands implements [launchr.CobraPlugin] interface to provide bump functionality.
func (p *Plugin) CobraAddCommands(rootCmd *launchr.Command) error {
	var source string

	var depCmd = &cobra.Command{
		Use:     "dependencies",
		Short:   "Shows dependencies and dependent resources of selected resource",
		Aliases: []string{"deps"},
		Args:    cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *launchr.Command, args []string) error {
			// Don't show usage help on a runtime error.
			cmd.SilenceUsage = true

			if _, err := os.Stat(source); os.IsNotExist(err) {
				launchr.Term().Warning().Printfln("%s doesn't exist, fallback to current dir", source)
				source = "."
			} else {
				launchr.Term().Info().Printfln("Selected source is %s", source)
			}

			showPaths, err := cmd.Flags().GetBool("mrn")
			if err != nil {
				return err
			}

			showTree, err := cmd.Flags().GetBool("tree")
			if err != nil {
				return err
			}

			depth, err := cmd.Flags().GetInt8("depth")
			if err != nil {
				return err
			}
			if depth == 0 {
				return fmt.Errorf("depth value should not be zero")
			}

			return dependencies(args[0], source, !showPaths, showTree, depth)
		},
	}

	depCmd.Flags().StringVar(&source, "source", ".compose/build", "Resources source dir")
	depCmd.Flags().Bool("mrn", false, "Show MRN instead of paths")
	depCmd.Flags().Bool("tree", false, "Show dependencies in tree-like output")
	depCmd.Flags().Int8("depth", 99, "Limit recursion lookup depth")
	depCmd.SetArgs([]string{"target"})

	rootCmd.AddCommand(depCmd)
	return nil
}

func isMachineResourceName(target string) bool {
	list := strings.Split(target, "__")
	return len(list) == 3
}

func convertTarget(source, target string) (string, error) {
	r := sync.BuildResourceFromPath(target, source)
	if r == nil {
		return "", fmt.Errorf("not valid resource %q", target)
	}

	return r.GetName(), nil
}

func convertToPath(mrn string) string {
	parts := strings.Split(mrn, "__")
	return filepath.Join(parts[0], parts[1], "roles", parts[2])
}

func dependencies(target, source string, toPath, showTree bool, depth int8) error {
	searchMrn := target
	if !isMachineResourceName(searchMrn) {
		converted, err := convertTarget(source, target)
		if err != nil {
			return err
		}

		searchMrn = converted
	}

	var header string
	if toPath {
		header = convertToPath(searchMrn)
	} else {
		header = searchMrn
	}

	// @TODO move inventory into dependencies?
	inv, err := sync.NewInventory(source)
	if err != nil {
		return err
	}
	requiredMap := inv.GetRequiredMap()
	parents := lookupDependencies(searchMrn, requiredMap, depth)
	if len(parents) > 0 {
		launchr.Term().Info().Println("Dependent resources:")
		if showTree {
			var parentsTree forwardTree = requiredMap
			parentsTree.print(header, "", 1, depth, searchMrn, toPath)
		} else {
			printList(parents, toPath)
		}
	}

	dependenciesMap := inv.GetDependenciesMap()
	children := lookupDependencies(searchMrn, dependenciesMap, depth)
	if len(children) > 0 {
		launchr.Term().Info().Println("Dependencies:")
		if showTree {
			var childrenTree forwardTree = dependenciesMap
			childrenTree.print(header, "", 1, depth, searchMrn, toPath)
		} else {
			printList(children, toPath)
		}
	}

	return nil
}

func printList(items map[string]bool, toPath bool) {
	keys := make([]string, 0, len(items))
	for k := range items {
		keys = append(keys, k)
	}

	sort.Strings(keys)
	for _, item := range keys {
		res := item
		if toPath {
			res = convertToPath(res)
		}

		launchr.Term().Print(res + "\n")
	}
}

func lookupDependencies(resourceName string, resourcesMap map[string]*sync.OrderedMap[bool], depth int8) map[string]bool {
	result := make(map[string]bool)
	if m, ok := resourcesMap[resourceName]; ok {
		for _, item := range m.Keys() {
			result[item] = true
			lookupDependenciesRecursively(item, resourcesMap, result, 1, depth)
		}
	}

	return result
}

func lookupDependenciesRecursively(resourceName string, resourcesMap map[string]*sync.OrderedMap[bool], result map[string]bool, depth, limit int8) {
	if depth == limit {
		return
	}

	if m, ok := resourcesMap[resourceName]; ok {
		for _, item := range m.Keys() {
			result[item] = true
			lookupDependenciesRecursively(item, resourcesMap, result, depth+1, limit)
		}
	}
}

type forwardTree map[string]*sync.OrderedMap[bool]

func (t forwardTree) print(header, indent string, depth, limit int8, parent string, toPath bool) {
	if indent == "" {
		launchr.Term().Printfln(header)
	}

	if depth == limit {
		return
	}

	children, ok := t[parent]
	if !ok {
		return
	}

	keys := children.Keys()
	sort.Strings(keys)

	for i, node := range keys {
		isLast := i == len(keys)-1
		var newIndent, edge string

		if isLast {
			newIndent = indent + "    "
			edge = "└── "
		} else {
			newIndent = indent + "│   "
			edge = "├── "
		}
		value := node
		if toPath {
			value = convertToPath(value)
		}

		launchr.Term().Printfln(indent + edge + value)
		t.print("", newIndent, depth+1, limit, node, toPath)
	}
}
