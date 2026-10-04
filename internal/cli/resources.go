package cli

import (
	"github.com/spf13/cobra"
)

func newResourcesCommand(rc *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "devices",
		Short: "Example device commands",
	}
	cmd.AddCommand(&cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List devices",
		RunE: func(cmd *cobra.Command, args []string) error {
			resources, err := rc.client.ListResources(cmd.Context())
			if err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(resources)
			}
			rows := make([][]string, 0, len(resources))
			for _, resource := range resources {
				rows = append(rows, []string{resource.ID, resource.Name, resource.Description})
			}
			rc.out.Table([]string{"ID", "NAME", "DESCRIPTION"}, rows)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "get <id>",
		Short: "Get one device",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			resource, err := rc.client.GetResource(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			if rc.out.IsJSON() {
				return rc.out.JSON(resource)
			}
			rc.out.Table([]string{"KEY", "VALUE"}, [][]string{
				{"id", resource.ID},
				{"name", resource.Name},
				{"description", resource.Description},
			})
			return nil
		},
	})
	return cmd
}
