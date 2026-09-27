// Command controller es el punto de entrada del controlador: corre
// indefinidamente ejecutando un ciclo de observación y decisión cada
// IntervaloEvaluacion. Se detiene con Ctrl+C (SIGINT) o SIGTERM.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"autoscaling-controller/internal/awsclient"
	"autoscaling-controller/internal/config"
	"autoscaling-controller/internal/inject"
	"autoscaling-controller/internal/logging"
	"autoscaling-controller/internal/policy"
	"autoscaling-controller/internal/state"
)

func main() {
	fmt.Println("== Auto-Scaling Controller ==")
	fmt.Printf("Intervalo=%s ventana=%s umbral_subir=%.0f%% umbral_bajar=%.0f%% min=%d max=%d\n",
		config.IntervaloEvaluacion, config.VentanaObservacion,
		config.CPUSubir, config.CPUBajar, config.MinInstancias, config.MaxInstancias)

	// Clientes de AWS: se construyen una sola vez y se reutilizan.
	ctxInicio, cancelarInicio := context.WithTimeout(context.Background(), 30*time.Second)
	clientes, err := awsclient.NuevosClientes(ctxInicio)
	cancelarInicio()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR fatal al inicializar AWS: %v\n", err)
		os.Exit(1)
	}

	est := state.Nuevo() // cooldowns en memoria

	// Parada ordenada ante Ctrl+C / SIGTERM.
	parar := make(chan os.Signal, 1)
	signal.Notify(parar, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Controlador en marcha. Ctrl+C para detener.")

	for {
		inicioCiclo := time.Now()
		ejecutarCiclo(clientes, est, inicioCiclo)

		// Descontar el tiempo gastado del intervalo, para no acumular atraso.
		espera := config.IntervaloEvaluacion - time.Since(inicioCiclo)
		if espera < 0 {
			espera = 0
		}
		select {
		case <-parar:
			fmt.Println("\nSeñal recibida. Deteniendo el controlador.")
			return
		case <-time.After(espera):
		}
	}
}

// ejecutarCiclo corre un ciclo: inventario -> métricas -> cooldown -> decidir
// -> actuar -> loguear. Un fallo se registra y el bucle continúa observando.
func ejecutarCiclo(clientes *awsclient.Clientes, est *state.Estado, ahora time.Time) {
	// Context acotado por ciclo, con margen para los reintentos.
	ctx, cancelar := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelar()

	ids, err := awsclient.ListManagedInstances(ctx, clientes.EC2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] ciclo abortado: fallo inventario: %v\n", ahora.Format(time.RFC3339), err)
		return
	}

	snap, err := awsclient.ObtenerSnapshot(ctx, clientes.CloudWatch, clientes.ELB, ids)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[%s] ciclo abortado: fallo metricas: %v\n", ahora.Format(time.RFC3339), err)
		return
	}

	// Modo de inyección para experimentos (no hace nada si no existe el archivo).
	if inject.Aplicar(&snap) {
		fmt.Fprintf(os.Stderr, "[%s] (inyeccion activa: cpu=%.1f%% latencia=%.2fs)\n",
			ahora.Format(time.RFC3339), snap.CPUMax, snap.LatenciaAvg)
	}

	snap.EnCooldown = est.EnCooldown(ahora)
	decision := policy.Decide(snap)

	// Actuar según la decisión; se registra el resultado para el log.
	resultado := "sin accion"
	switch decision.Accion {
	case policy.Subir:
		id, err := awsclient.LanzarInstancia(ctx, clientes.EC2, clientes.ELB)
		if err != nil {
			resultado = "ERROR al lanzar: " + err.Error()
		} else {
			resultado = "instancia lanzada y registrada " + id
			est.RegistrarSubida(ahora)
		}
	case policy.Bajar:
		id, err := awsclient.TerminarInstancia(ctx, clientes.EC2, clientes.ELB, ids)
		if err != nil {
			resultado = "ERROR al terminar: " + err.Error()
		} else {
			resultado = "instancia desregistrada y terminada " + id
			est.RegistrarBajada(ahora)
		}
	}

	logging.RegistrarCiclo(ahora, snap, decision, resultado)
}
