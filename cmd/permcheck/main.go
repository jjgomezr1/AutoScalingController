// Command permcheck verifica que las llamadas a AWS funcionan: lista las
// instancias gestionadas y arma el Snapshot de métricas. No actúa sobre nada.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"autoscaling-controller/internal/awsclient"
	"autoscaling-controller/internal/config"
)

func main() {
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()

	fmt.Println("== permcheck: verificacion de acceso a AWS ==")

	clientes, err := awsclient.NuevosClientes(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR cargando configuracion/credenciales de AWS: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Clientes de AWS creados correctamente (credenciales y region OK).")

	fmt.Printf("\nBuscando instancias con tag %s=%s en estado running...\n",
		config.TagClave, config.TagValor)
	ids, err := awsclient.ListManagedInstances(ctx, clientes.EC2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR consultando EC2: %v\n", err)
		os.Exit(1)
	}
	if len(ids) == 0 {
		fmt.Println("Resultado: 0 instancias gestionadas corriendo.")
	} else {
		fmt.Printf("Resultado: %d instancia(s) gestionada(s) corriendo:\n", len(ids))
		for _, id := range ids {
			fmt.Printf("  - %s\n", id)
		}
	}

	fmt.Printf("\nConsultando metricas en CloudWatch (ventana %s, punto %ds)...\n",
		config.VentanaObservacion, config.PeriodoPuntoSegundos)
	snap, err := awsclient.ObtenerSnapshot(ctx, clientes.CloudWatch, clientes.ELB, ids)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR consultando CloudWatch: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Snapshot construido:")
	fmt.Printf("  Instancias corriendo : %d\n", snap.InstanciasCorriendo)
	fmt.Printf("  CPU maxima           : %.2f%%\n", snap.CPUMax)
	fmt.Printf("  Latencia             : %.3fs\n", snap.LatenciaAvg)
	fmt.Printf("  Hosts saludables     : %d\n", snap.HostsSaludables)
	fmt.Printf("  Metrica confiable    : %t\n", snap.MetricaConfiable)
	fmt.Printf("  En cooldown          : %t\n", snap.EnCooldown)

	if !snap.MetricaConfiable {
		fmt.Println("  (Nota: metrica no confiable; posiblemente aun no hay dato reciente en la ventana.)")
	}
}
