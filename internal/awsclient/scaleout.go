package awsclient

import (
	"context"
	"fmt"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"autoscaling-controller/internal/config"
)

// LanzarInstancia crea una instancia desde la AMI dorada, la etiqueta y la
// registra en el Target Group. El ALB no le envía tráfico hasta que su health
// check la marque healthy, así que registrarla de una es seguro.
func LanzarInstancia(ctx context.Context, cEC2 *ec2.Client, cELB *elbv2.Client) (string, error) {
	entrada := &ec2.RunInstancesInput{
		ImageId:      awssdk.String(config.AMIImagen),
		InstanceType: ec2types.InstanceType(config.TipoInstancia),
		KeyName:      awssdk.String(config.KeyPair),
		MinCount:     awssdk.Int32(1), // siempre una por decisión
		MaxCount:     awssdk.Int32(1),
		SubnetId:         awssdk.String(config.SubnetID),
		SecurityGroupIds: []string{config.SecurityGroupID},
		// Monitoreo detallado: CPU cada 1 min (necesario para la ventana de 4 min).
		Monitoring: &ec2types.RunInstancesMonitoringEnabled{Enabled: awssdk.Bool(true)},
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{Key: awssdk.String(config.TagClave), Value: awssdk.String(config.TagValor)},
					{Key: awssdk.String("Name"), Value: awssdk.String("sujeto-autoscaling")},
				},
			},
		},
	}

	salida, err := ReintentarConValor(ctx, "RunInstances", func() (*ec2.RunInstancesOutput, error) {
		return cEC2.RunInstances(ctx, entrada)
	})
	if err != nil {
		return "", fmt.Errorf("fallo al lanzar la instancia (RunInstances): %w", err)
	}
	if len(salida.Instances) == 0 || salida.Instances[0].InstanceId == nil {
		return "", fmt.Errorf("RunInstances no devolvio ninguna instancia")
	}
	id := *salida.Instances[0].InstanceId

	err = Reintentar(ctx, "RegisterTargets", func() error {
		_, e := cELB.RegisterTargets(ctx, &elbv2.RegisterTargetsInput{
			TargetGroupArn: awssdk.String(config.TargetGroupARN),
			Targets:        []elbtypes.TargetDescription{{Id: awssdk.String(id)}},
		})
		return e
	})
	if err != nil {
		// La instancia ya se lanzó (queda con el tag); solo falló el registro.
		return id, fmt.Errorf("instancia %s lanzada pero fallo al registrarla en el target group: %w", id, err)
	}

	return id, nil
}
