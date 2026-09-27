// Package awsclient agrupa todo el código que habla con AWS. Ninguna otra
// parte del controlador importa el SDK directamente.
package awsclient

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
)

// Clientes agrupa los clientes de AWS; se construyen una vez y se reutilizan.
type Clientes struct {
	EC2        *ec2.Client
	CloudWatch *cloudwatch.Client
	ELB        *elbv2.Client
}

// NuevosClientes carga la config del SDK (credenciales automáticas vía IAM
// Instance Profile o AWS CLI) y construye los clientes. Región: us-east-1.
func NuevosClientes(ctx context.Context) (*Clientes, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("no se pudo cargar la configuracion de AWS: %w", err)
	}

	return &Clientes{
		EC2:        ec2.NewFromConfig(cfg),
		CloudWatch: cloudwatch.NewFromConfig(cfg),
		ELB:        elbv2.NewFromConfig(cfg),
	}, nil
}
