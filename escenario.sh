#!/usr/bin/env bash
# Escenario de carga sintetico: escribe valores de CPU en el archivo de
# inyeccion que el controller lee cada ciclo. Uso: ./escenario.sh

ARCHIVO="${INYECCION_FILE:-/home/ubuntu/inyeccion.txt}"

# "CPU segundos" por fase: subida, pico, meseta, bajada, valle.
FASES=(
  "20 240"
  "75 360"
  "85 480"
  "40 360"
  "15 480"
  "10 480"
)

echo "== escenario de carga sintetico =="
echo "archivo de inyeccion: $ARCHIVO"
echo "inicio: $(date -u +%FT%TZ)"

for fase in "${FASES[@]}"; do
  cpu="${fase%% *}"
  dur="${fase##* }"
  echo "$cpu" > "$ARCHIVO"
  echo "[$(date -u +%FT%TZ)] CPU sintetica = ${cpu}%  durante ${dur}s"
  sleep "$dur"
done

rm -f "$ARCHIVO"
echo "[$(date -u +%FT%TZ)] escenario terminado."
