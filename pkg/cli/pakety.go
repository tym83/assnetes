package cli

import (
	"fmt"

	kubepkgcli "github.com/kuberoot-dev/kubepkg/pkg/cli"
	"github.com/spf13/cobra"
)

// GroupPakety is the API group of packages, «пакеты.бух».
const GroupPakety = "xn--80akoss8c.xn--90a0bg"

// Russian names of the kubepkg commands, the way an accountant buys things.
var paketyNames = map[string][]string{
	"install":  {"закупить"},
	"remove":   {"списать"},
	"list":     {"остатки"},
	"search":   {"прайс"},
	"plan":     {"смета"},
	"history":  {"история"},
	"rollback": {"сторно"},
	"repo":     {"поставщики"},
	"adopt":    {"оприходовать"},
}

// newPaketyCommand embeds kubepkg as «1сctl пакеты» under the API group of
// the distribution, with Russian names, and the everyday ones on top:
// «1сctl закупить бухгалтерия».
func newPaketyCommand(root *cobra.Command) {
	opts := kubepkgcli.DefaultOptions()
	opts.Name = "пакеты"
	opts.Short = "пакетный менеджер: закупить, списать, остатки, прайс, смета, сторно"
	opts.APIGroup = GroupPakety
	pk := kubepkgcli.NewRootCommand(opts)
	pk.Aliases = []string{"packages"}
	for _, c := range pk.Commands() {
		if ru, ok := paketyNames[c.Name()]; ok {
			c.Aliases = append(c.Aliases, ru...)
		}
	}
	root.AddCommand(pk)
	for _, en := range []string{"install", "remove", "list", "search", "rollback"} {
		ru := paketyNames[en][0]
		target := en
		root.AddCommand(&cobra.Command{
			Use:                ru,
			Short:              fmt.Sprintf("то же, что «1сctl пакеты %s»", ru),
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				pk.SetArgs(append([]string{target}, args...))
				return pk.ExecuteContext(cmd.Context())
			},
		})
	}
}
