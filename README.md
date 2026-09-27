### Auto Scaling Controller

Controlador de auto-scaling que no usa Auto Scaling Group, toma decisiones propias a partir su propia politica con las metricas obtenidas desde AWS. Gestiona entre 1 y 5 instancias de EC2 que cuentan con una aplicacion web.

### Requisitos

Go 1.24+ para compilar los programas del controller.

### Explicacion de la estructura:

cmd/
  controller: Bucle del controlador.
  scaletest: Prueba funcional con metricas inyectadas.
  permcheck: verificar permisos y que pueda leer las metricas.
internal/
  config: Umbrales y parametros.
  policy: Logica de decision con pruebas.
  awsclient: Comunicacion con AWS.
  state: Cooldowns en memoria (guardan el tiempo de la ultima accion).
  logging: Registro estructurado de cada ciclo.
  inject: Inyeccion de metricas sinteticas para pruebas.
escenario.sh: Generador del escenario de carga.
controller.service: servicio systemd para que funcione al activarse el lab.

## Implementacion:

Region de trabajo en todos los pasos: us-east-1.

1. Crear la app y la AMI dorada.

1.1 Lanzar la instancia base.
- Servicio EC2 → "Instances" → "Launch instances":
  - Name: plantilla-app
  - Application and OS Images: Ubuntu Server 24.04 LTS (HVM), SSD Volume Type, 64-bit (x86).
  - Instance type: t3.micro.
  - Key pair (login): vockey.
  - Network settings (Edit):
    - Auto-assign Public IP: Enable.
    - Firewall (security groups): crear uno nuevo que permita SSH (22) desde Anywhere-IPv4.
  - No agregue etiquetas.
- "Launch instance". Espere a Running con 2/2 checks.

1.2 Instalar la app como servicio.
- Seleccione la instancia → "Connect" → pestaña "EC2 Instance Connect" → usuario ubuntu → "Connect".
- En la terminal:
  ```
  sudo apt update
  sudo apt install -y python3-pip python3-venv git
  cd /home/ubuntu
  git clone https://github.com/jjgomezr1/AppReverseText.git app
  cd app
  python3 -m venv venv
  ./venv/bin/pip install -r requirements.txt
  sudo cp inversor.service /etc/systemd/system/inversor.service
  sudo systemctl daemon-reload
  sudo systemctl enable inversor
  sudo systemctl start inversor
  curl -i http://localhost/
  ```
  El curl debe devolver HTTP/1.1 200 OK.

1.3 Crear la AMI dorada.
- EC2 → Instances → seleccione plantilla-app → "Actions" → "Image and templates" → "Create image":
  - Image name: ami-inversor-app
- "Create image". Espere en "AMIs" a estado Available. Anote el AMI ID.
- Termine la instancia plantilla-app.

2. Crear la VPC y la red.
- Servicio VPC → "Your VPCs" → "Create VPC" → opcion "VPC and more":
  - Name tag auto-generation: autoscaling
  - IPv4 CIDR block: 10.0.0.0/16
  - Number of Availability Zones (AZs): 2
  - Number of public subnets: 2
  - Number of private subnets: 0
  - NAT gateways: None
  - VPC endpoints: None
- "Create VPC". Anote el VPC ID y los IDs de las 2 subredes publicas.

3. Crear los security groups.

3.1 Security group del ALB.
- VPC → "Security" → "Security Groups" → "Create security group":
  - Security group name: sg-alb
  - VPC: autoscaling
  - Inbound rules → "Add rule": Type HTTP, Source Anywhere-IPv4.
- "Create security group". Anote su ID.

3.2 Security group de las instancias.
- "Create security group":
  - Security group name: sg-instancias
  - VPC: autoscaling
  - Inbound rules:
    - "Add rule": Type HTTP, Source Custom → sg-alb.
    - "Add rule": Type SSH, Source Anywhere-IPv4.
- "Create security group". Anote su ID.

4. Crear el Target Group.
- EC2 → "Target Groups" → "Create target group":
  - Target type: Instances
  - Target group name: tg-InversorApp
  - Protocol / Port: HTTP / 80
  - VPC: autoscaling
  - Health checks: Protocol HTTP, Path /
  - Advanced health check settings: Healthy threshold 2, Unhealthy threshold 2, Timeout 5, Interval 15, Success codes 200.
- "Next" (sin registrar instancias) → "Create target group".
- Seleccione tg-InversorApp → pestaña "Attributes" → "Edit" → Deregistration delay: 30 → guardar.
- Anote el ARN del Target Group.

5. Crear el Application Load Balancer.
- EC2 → "Load Balancers" → "Create load balancer" → "Application Load Balancer":
  - Load balancer name: alb-InversorApp
  - Scheme: Internet-facing
  - IP address type: IPv4
  - Network mapping: VPC autoscaling; seleccione las 2 AZ y en cada una su subred publica.
  - Security groups: sg-alb (quite el default).
  - Listeners and routing: Listener HTTP : 80 → Forward to → tg-InversorApp.
- "Create load balancer". Espere a estado Active.

6. Crear la instancia controladora.
- EC2 → "Launch instances":
  - Name: controlador
  - AMI: Ubuntu Server 24.04 LTS (64-bit x86)
  - Instance type: t3.micro
  - Key pair: vockey
  - Network settings (Edit):
    - VPC: autoscaling
    - Subnet: una subred publica
    - Auto-assign Public IP: Enable
    - Firewall: un security group que permita SSH (22).
  - Advanced details → IAM instance profile: LabInstanceProfile.
  - No agregue el tag ManagedBy.
- "Launch instance". Espere a Running con 2/2 checks. Anote su IP publica.

7. Configurar los parametros del controller. (OBLIGATORIO antes de compilar y desplegar)
- Clone el repositorio del controller en su equipo local:
  ```
  git clone https://github.com/jjgomezr1/AutoScalingController.git
  cd AutoScalingController
  ```
- El archivo controller.service trae placeholders con el formato
  <AQUI INSERTAR ...>. Reemplace CADA UNO por el recurso real anotado en los
  pasos anteriores (seccion [Service], lineas Environment=). Si no lo hace, el
  controller fallara al intentar usar valores invalidos:
  - AMI_ID: AMI del paso 1.3
  - SUBNET_ID: subred publica del paso 2
  - SECURITY_GROUP: sg-instancias del paso 3.2
  - TARGET_GROUP_ARN: ARN del paso 4
  - ALB_NOMBRE: alb-InversorApp
  - TIPO_INSTANCIA: t3.micro (dejar tal cual)
  - KEY_PAIR: vockey (dejar tal cual)
- Verifique que no quede ningun placeholder <AQUI INSERTAR ...> en el archivo
  antes de continuar.

8. Compilar el controller.
- En la carpeta del repositorio clonado, desde Windows PowerShell:
  ```
  $env:GOOS="linux"; $env:GOARCH="amd64"
  go build -o controller ./cmd/controller
  Remove-Item Env:GOOS; Remove-Item Env:GOARCH
  ```
- (Opcional) Pruebas unitarias:
  ```
  go test ./internal/policy/
  ```

9. Desplegar el controller.
- Suba el binario y el servicio (reemplace la ruta del .pem y la IP del paso 6):
  ```
  scp -i "ruta/labsuser.pem" controller         ubuntu@IP_CONTROLADOR:/home/ubuntu/
  scp -i "ruta/labsuser.pem" controller.service ubuntu@IP_CONTROLADOR:/home/ubuntu/
  ```
- Conectese a la controladora (EC2 Instance Connect, usuario ubuntu):
  ```
  cd ~
  chmod +x controller
  sudo cp controller.service /etc/systemd/system/controller.service
  sudo systemctl daemon-reload
  sudo systemctl enable controller
  sudo systemctl start controller
  sudo systemctl status controller --no-pager
  ```

10. Ejecucion y monitoreo.
- Ver los logs en vivo:
  ```
  journalctl -u controller -f
  ```
- Detener o reiniciar:
  ```
  sudo systemctl stop controller
  sudo systemctl restart controller
  ```

11. Correr el experimento con metricas sinteticas.
- Detenga el servicio para correr el controller en primer plano y capturar el log:
  ```
  sudo systemctl stop controller
  ```
- Compile y suba el escenario (desde su equipo local, mismo repositorio):
  ```
  scp -i "ruta/labsuser.pem" escenario.sh ubuntu@IP_CONTROLADOR:/home/ubuntu/
  ```
- En la controladora, en una terminal, arranque el controller guardando el log:
  ```
  cd ~
  chmod +x controller escenario.sh
  ./controller > experimento.log 2>&1
  ```
- En una segunda terminal, lance el escenario:
  ```
  ./escenario.sh
  ```
- El escenario dura ~40 min y escribe valores de CPU sinteticos en inyeccion.txt, que el controller lee cada ciclo. Al terminar borra el archivo y el controller vuelve a metricas reales.
- Al finalizar, detenga el controller (Ctrl+C) y descargue el log (desde su equipo local):
  ```
  scp -i "ruta/labsuser.pem" ubuntu@IP_CONTROLADOR:/home/ubuntu/experimento.log .
  ```

12. Limpieza.
- Detenga el servicio y termine las instancias sujeto desde la consola EC2. Para no gastar creditos, elimine tambien el ALB, el Target Group, la AMI y la VPC cuando ya no los use.
