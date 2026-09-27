// Package config centraliza umbrales, cooldowns y parámetros de despliegue.
// Es la única fuente de verdad de la configuración del controlador.
package config

import (
	"os"
	"time"
)

// getenv devuelve la variable de entorno, o el valor por defecto si no existe.
func getenv(clave, porDefecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return porDefecto
}

// Límites de capacidad (impuestos por el reto).
const (
	MinInstancias = 1
	MaxInstancias = 5
)

// Umbrales de decisión (%CPU y segundos de latencia). La banda muerta entre
// subir y bajar evita oscilación. CPU sube en 60% por la naturaleza burstable
// de la t3.micro.
const (
	CPUSubir      = 60.0
	CPUBajar      = 30.0
	LatenciaSubir = 1.0
	LatenciaBajar = 0.4
)

// Ventana de observación y consulta de métricas.
const (
	VentanaObservacion = 4 * time.Minute
	// Granularidad de cada punto pedido a CloudWatch. 60s coincide con el
	// monitoreo detallado de EC2 y da puntos frescos dentro de la ventana.
	PeriodoPuntoSegundos = 60
	// Si el dato más reciente supera esta antigüedad, se considera no confiable.
	MaxAntiguedadMetrica = 2 * time.Minute
)

// Cooldowns anti-oscilación. El de bajada es mayor porque quedar corto de
// capacidad afecta al usuario de inmediato; sobrar capacidad, no.
const (
	CooldownSubida = 3 * time.Minute
	CooldownBajada = 6 * time.Minute
)

// Draining: al bajar se desregistra el target, se espera a que salga de
// "draining" (sondeando) y luego se termina la instancia.
const (
	EsperaMaxDraining       = 45 * time.Second
	IntervaloSondeoDraining = 5 * time.Second
)

// Ritmo del ciclo y reintentos ante fallos de AWS.
const (
	IntervaloEvaluacion   = 1 * time.Minute
	MaxReintentos         = 2
	EsperaEntreReintentos = 20 * time.Second
)

// Tag que agrupa las instancias gestionadas (no usamos Auto Scaling Group).
const (
	TagClave = "ManagedBy"
	TagValor = "autoscaling-controller"
)

// Parámetros de infraestructura. Sobreescribibles por variable de entorno,
// ya que estos IDs cambian si se recrea la infraestructura.
var (
	TipoInstancia   = getenv("TIPO_INSTANCIA", "t3.micro")
	KeyPair         = getenv("KEY_PAIR", "vockey")
	// AMI dorada con la app ya instalada como servicio (arranca sola al bootear).
	AMIImagen       = getenv("AMI_ID", "<AQUI INSERTAR ID (AMI DORADA)>")
	SubnetID        = getenv("SUBNET_ID", "<AQUI INSERTAR ID (SUBRED PUBLICA)>")
	SecurityGroupID = getenv("SECURITY_GROUP", "<AQUI INSERTAR ID (SECURITY GROUP INSTANCIAS)>")
	TargetGroupARN  = getenv("TARGET_GROUP_ARN", "<AQUI INSERTAR ARN (TARGET GROUP)>")
	// Nombre del ALB, usado como dimensión al consultar TargetResponseTime.
	ALBNombre = getenv("ALB_NOMBRE", "<AQUI INSERTAR NOMBRE (ALB)>")
)
