package awsclient

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"autoscaling-controller/internal/config"
)

// ListManagedInstances devuelve los IDs de las instancias con el tag
// ManagedBy=autoscaling-controller en estado running.
func ListManagedInstances(ctx context.Context, cliente *ec2.Client) ([]string, error) {
	return ReintentarConValor(ctx, "inventario EC2", func() ([]string, error) {
		return listarInstancias(ctx, cliente)
	})
}

func listarInstancias(ctx context.Context, cliente *ec2.Client) ([]string, error) {
	// Filtramos del lado de AWS: por tag y por estado running.
	entrada := &ec2.DescribeInstancesInput{
		Filters: []types.Filter{
			{Name: awssdk.String("tag:" + config.TagClave), Values: []string{config.TagValor}},
			{Name: awssdk.String("instance-state-name"), Values: []string{"running"}},
		},
	}

	// El paginator recorre todas las páginas para no perder instancias.
	paginator := ec2.NewDescribeInstancesPaginator(cliente, entrada)

	var ids []string
	for paginator.HasMorePages() {
		pagina, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("fallo al describir instancias en EC2: %w", err)
		}
		// EC2 agrupa las instancias en Reservations; de ahí el doble bucle.
		for _, reserva := range pagina.Reservations {
			for _, instancia := range reserva.Instances {
				if instancia.InstanceId != nil {
					ids = append(ids, *instancia.InstanceId)
				}
			}
		}
	}

	return ids, nil
}
