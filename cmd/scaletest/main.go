// Command scaletest ejercita el pipeline (inventario -> métricas -> decidir ->
// actuar) de forma controlada, con inyección opcional de CPU/latencia. Sin
// -actuar es dry-run (solo informa).
//
//	./scaletest                  # métricas reales, dry-run
//	./scaletest -cpu 85          # simula CPU 85%, dry-run
//	./scaletest -cpu 85 -actuar  # simula CPU 85% y ejecuta (lanza real)
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"autoscaling-controller/internal/awsclient"
	"autoscaling-controller/internal/policy"
)

func main() {
	cpuInyectada := flag.Float64("cpu", -1, "CPU sintetica (0-100); omitir usa la real")
	latInyectada := flag.Float64("latencia", -1, "latencia sintetica (s); omitir usa la real")
	actuar := flag.Bool("actuar", false, "ejecutar la accion real; sin el, solo informa")
	flag.Parse()

	ctx, cancelar := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelar()

	fmt.Println("== scaletest: prueba controlada del pipeline de escalado ==")

	clientes, err := awsclient.NuevosClientes(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR cargando AWS: %v\n", err)
		os.Exit(1)
	}

	ids, err := awsclient.ListManagedInstances(ctx, clientes.EC2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR inventario: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Instancias gestionadas corriendo: %d\n", len(ids))
	for _, id := range ids {
		fmt.Printf("  - %s\n", id)
	}

	snap, err := awsclient.ObtenerSnapshot(ctx, clientes.CloudWatch, clientes.ELB, ids)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR metricas: %v\n", err)
		os.Exit(1)
	}

	// Inyección opcional (marca la métrica como confiable).
	if *cpuInyectada >= 0 {
		fmt.Printf("\n[INYECCION] CPU real %.2f%% -> sintetica %.2f%%\n", snap.CPUMax, *cpuInyectada)
		snap.CPUMax = *cpuInyectada
		snap.MetricaConfiable = true
	}
	if *latInyectada >= 0 {
		fmt.Printf("[INYECCION] Latencia real %.3fs -> sintetica %.3fs\n", snap.LatenciaAvg, *latInyectada)
		snap.LatenciaAvg = *latInyectada
		snap.MetricaConfiable = true
	}

	fmt.Println("\nSnapshot para decidir:")
	fmt.Printf("  Instancias corriendo : %d\n", snap.InstanciasCorriendo)
	fmt.Printf("  CPU maxima           : %.2f%%\n", snap.CPUMax)
	fmt.Printf("  Latencia             : %.3fs\n", snap.LatenciaAvg)
	fmt.Printf("  Hosts saludables     : %d\n", snap.HostsSaludables)
	fmt.Printf("  Metrica confiable    : %t\n", snap.MetricaConfiable)
	fmt.Printf("  En cooldown          : %t\n", snap.EnCooldown)

	decision := policy.Decide(snap)
	fmt.Printf("\nDecision: %s\n", decision.Accion)
	fmt.Printf("Motivo  : %s\n", decision.Motivo)

	if !*actuar {
		fmt.Println("\n(dry-run: no se ejecuta ninguna accion. Pasa -actuar para ejecutar.)")
		return
	}

	switch decision.Accion {
	case policy.Subir:
		fmt.Println("\n[ACTUAR] Lanzando y registrando una instancia...")
		id, err := awsclient.LanzarInstancia(ctx, clientes.EC2, clientes.ELB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR al lanzar: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Instancia lanzada: %s\n", id)
	case policy.Bajar:
		fmt.Println("\n[ACTUAR] Desregistrando, drenando y terminando una instancia...")
		id, err := awsclient.TerminarInstancia(ctx, clientes.EC2, clientes.ELB, ids)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR al terminar: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Instancia terminada: %s\n", id)
	default:
		fmt.Println("\n[ACTUAR] La decision es mantener; no hay nada que ejecutar.")
	}
}
