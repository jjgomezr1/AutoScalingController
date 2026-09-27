package awsclient

import (
	"context"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"
	elbtypes "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2/types"

	"autoscaling-controller/internal/config"
)

// elegirInstanciaATerminar devuelve la instancia MÁS NUEVA (mayor LaunchTime):
// al reducir carga se deshace el último scale-out, conservando las veteranas.
func elegirInstanciaATerminar(ctx context.Context, cEC2 *ec2.Client, ids []string) (string, error) {
	salida, err := ReintentarConValor(ctx, "DescribeInstances (LaunchTime)", func() (*ec2.DescribeInstancesOutput, error) {
		return cEC2.DescribeInstances(ctx, &ec2.DescribeInstancesInput{InstanceIds: ids})
	})
	if err != nil {
		return "", fmt.Errorf("fallo al consultar LaunchTime de las instancias: %w", err)
	}

	var elegido string
	var lanzamientoElegido time.Time
	for _, reserva := range salida.Reservations {
		for _, inst := range reserva.Instances {
			if inst.InstanceId == nil || inst.LaunchTime == nil {
				continue
			}
			if elegido == "" || inst.LaunchTime.After(lanzamientoElegido) {
				elegido = *inst.InstanceId
				lanzamientoElegido = *inst.LaunchTime
			}
		}
	}

	if elegido == "" {
		return "", fmt.Errorf("no se pudo determinar una instancia a terminar")
	}
	return elegido, nil
}

// esperarDraining sondea el estado del target hasta que deja de estar
// "draining" (o hasta agotar EsperaMaxDraining).
func esperarDraining(ctx context.Context, cELB *elbv2.Client, id string) {
	limite := time.Now().Add(config.EsperaMaxDraining)
	for time.Now().Before(limite) {
		salida, err := cELB.DescribeTargetHealth(ctx, &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: awssdk.String(config.TargetGroupARN),
			Targets:        []elbtypes.TargetDescription{{Id: awssdk.String(id)}},
		})
		if err == nil {
			if len(salida.TargetHealthDescriptions) == 0 {
				return // ya no está en el grupo: draining completo
			}
			if salida.TargetHealthDescriptions[0].TargetHealth.State != elbtypes.TargetHealthStateEnumDraining {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(config.IntervaloSondeoDraining):
		}
	}
}

// TerminarInstancia hace la bajada segura: elige la instancia más nueva, la
// desregistra del Target Group, espera el draining y la termina. Respeta el
// mínimo como guarda defensiva.
func TerminarInstancia(ctx context.Context, cEC2 *ec2.Client, cELB *elbv2.Client, ids []string) (string, error) {
	if len(ids) <= config.MinInstancias {
		return "", fmt.Errorf("no se termina: hay %d instancia(s) y el minimo es %d", len(ids), config.MinInstancias)
	}

	elegido, err := elegirInstanciaATerminar(ctx, cEC2, ids)
	if err != nil {
		return "", err
	}

	// Desregistrar: deja de recibir tráfico nuevo y entra en "draining".
	err = Reintentar(ctx, "DeregisterTargets", func() error {
		_, e := cELB.DeregisterTargets(ctx, &elbv2.DeregisterTargetsInput{
			TargetGroupArn: awssdk.String(config.TargetGroupARN),
			Targets:        []elbtypes.TargetDescription{{Id: awssdk.String(elegido)}},
		})
		return e
	})
	if err != nil {
		return "", fmt.Errorf("fallo al desregistrar la instancia %s del target group: %w", elegido, err)
	}

	esperarDraining(ctx, cELB, elegido)

	err = Reintentar(ctx, "TerminateInstances", func() error {
		_, e := cEC2.TerminateInstances(ctx, &ec2.TerminateInstancesInput{InstanceIds: []string{elegido}})
		return e
	})
	if err != nil {
		return "", fmt.Errorf("fallo al terminar la instancia %s: %w", elegido, err)
	}

	return elegido, nil
}
