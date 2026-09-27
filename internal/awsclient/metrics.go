package awsclient

import (
	"context"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cwtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	elbv2 "github.com/aws/aws-sdk-go-v2/service/elasticloadbalancingv2"

	"autoscaling-controller/internal/config"
	"autoscaling-controller/internal/policy"
)

// dimensionTargetGroup extrae la dimensión "TargetGroup" del ARN del target
// group (la parte desde "targetgroup/").
func dimensionTargetGroup(arnTG string) string {
	i := strings.Index(arnTG, "targetgroup/")
	if i < 0 {
		return ""
	}
	return arnTG[i:]
}

// dimensionLoadBalancer resuelve la dimensión "LoadBalancer" del ALB (la parte
// "app/nombre/hash" de su ARN), consultándolo por nombre.
func dimensionLoadBalancer(ctx context.Context, cELB *elbv2.Client) (string, error) {
	salida, err := ReintentarConValor(ctx, "DescribeLoadBalancers", func() (*elbv2.DescribeLoadBalancersOutput, error) {
		return cELB.DescribeLoadBalancers(ctx, &elbv2.DescribeLoadBalancersInput{Names: []string{config.ALBNombre}})
	})
	if err != nil {
		return "", fmt.Errorf("no se pudo describir el ALB %q: %w", config.ALBNombre, err)
	}
	if len(salida.LoadBalancers) == 0 || salida.LoadBalancers[0].LoadBalancerArn == nil {
		return "", fmt.Errorf("no se encontro el ALB %q", config.ALBNombre)
	}
	arn := *salida.LoadBalancers[0].LoadBalancerArn
	i := strings.Index(arn, "app/")
	if i < 0 {
		return "", fmt.Errorf("ARN de ALB con formato inesperado: %s", arn)
	}
	return arn[i:], nil
}

// ObtenerSnapshot arma el Snapshot con las tres métricas: CPUMax
// (máximo entre instancias del promedio de CPU), LatenciaAvg (TargetResponseTime)
// y HostsSaludables (HealthyHostCount). Sin instancias, no consulta y marca la
// métrica como no confiable.
func ObtenerSnapshot(ctx context.Context, cCW *cloudwatch.Client, cELB *elbv2.Client, ids []string) (policy.Snapshot, error) {
	snap := policy.Snapshot{InstanciasCorriendo: len(ids)}

	if len(ids) == 0 {
		snap.MetricaConfiable = false
		return snap, nil
	}

	ahora := time.Now()
	inicio := ahora.Add(-config.VentanaObservacion)

	dimLB, err := dimensionLoadBalancer(ctx, cELB)
	if err != nil {
		return policy.Snapshot{}, err
	}
	dimTG := dimensionTargetGroup(config.TargetGroupARN)

	// Todas las métricas en una sola llamada GetMetricData.
	var consultas []cwtypes.MetricDataQuery

	// CPU por instancia (m0, m1, ...).
	for i, id := range ids {
		consultas = append(consultas, cwtypes.MetricDataQuery{
			Id: awssdk.String(fmt.Sprintf("m%d", i)),
			MetricStat: &cwtypes.MetricStat{
				Metric: &cwtypes.Metric{
					Namespace:  awssdk.String("AWS/EC2"),
					MetricName: awssdk.String("CPUUtilization"),
					Dimensions: []cwtypes.Dimension{{Name: awssdk.String("InstanceId"), Value: awssdk.String(id)}},
				},
				Period: awssdk.Int32(config.PeriodoPuntoSegundos),
				Stat:   awssdk.String("Average"),
			},
		})
	}

	// Latencia (TargetResponseTime).
	consultas = append(consultas, cwtypes.MetricDataQuery{
		Id: awssdk.String("lat"),
		MetricStat: &cwtypes.MetricStat{
			Metric: &cwtypes.Metric{
				Namespace:  awssdk.String("AWS/ApplicationELB"),
				MetricName: awssdk.String("TargetResponseTime"),
				Dimensions: []cwtypes.Dimension{{Name: awssdk.String("LoadBalancer"), Value: awssdk.String(dimLB)}},
			},
			Period: awssdk.Int32(config.PeriodoPuntoSegundos),
			Stat:   awssdk.String("Average"),
		},
	})

	// Hosts saludables (HealthyHostCount).
	consultas = append(consultas, cwtypes.MetricDataQuery{
		Id: awssdk.String("hosts"),
		MetricStat: &cwtypes.MetricStat{
			Metric: &cwtypes.Metric{
				Namespace:  awssdk.String("AWS/ApplicationELB"),
				MetricName: awssdk.String("HealthyHostCount"),
				Dimensions: []cwtypes.Dimension{
					{Name: awssdk.String("TargetGroup"), Value: awssdk.String(dimTG)},
					{Name: awssdk.String("LoadBalancer"), Value: awssdk.String(dimLB)},
				},
			},
			Period: awssdk.Int32(config.PeriodoPuntoSegundos),
			Stat:   awssdk.String("Maximum"),
		},
	})

	entrada := &cloudwatch.GetMetricDataInput{
		StartTime:         awssdk.Time(inicio),
		EndTime:           awssdk.Time(ahora),
		MetricDataQueries: consultas,
		ScanBy:            cwtypes.ScanByTimestampDescending, // [0] = más reciente
	}

	salida, err := ReintentarConValor(ctx, "GetMetricData", func() (*cloudwatch.GetMetricDataOutput, error) {
		return cCW.GetMetricData(ctx, entrada)
	})
	if err != nil {
		return policy.Snapshot{}, fmt.Errorf("fallo al consultar metricas en CloudWatch: %w", err)
	}

	var cpuMax, latencia float64
	var hosts int
	var huboCPU bool
	var cpuMasReciente time.Time

	for _, r := range salida.MetricDataResults {
		id := awssdk.ToString(r.Id)
		if len(r.Values) == 0 {
			continue
		}
		valorReciente := r.Values[0]

		switch {
		case strings.HasPrefix(id, "m"): // CPU de una instancia -> máximo
			if valorReciente > cpuMax {
				cpuMax = valorReciente
			}
			huboCPU = true
			if r.Timestamps[0].After(cpuMasReciente) {
				cpuMasReciente = r.Timestamps[0]
			}
		case id == "lat":
			latencia = valorReciente
		case id == "hosts":
			hosts = int(valorReciente)
		}
	}

	// La confiabilidad se ancla en la CPU (métrica por instancia, la crítica).
	snap.CPUMax = cpuMax
	snap.LatenciaAvg = latencia
	snap.HostsSaludables = hosts
	snap.MetricaConfiable = huboCPU && ahora.Sub(cpuMasReciente) <= config.MaxAntiguedadMetrica

	return snap, nil
}
