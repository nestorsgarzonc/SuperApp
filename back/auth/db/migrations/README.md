# Migraciones

Aplicar en orden ascendente con `go run ./cmd/setup` desde `back/auth/ms` o `make setup`. El setup registra cada migración/seed aplicada y toma un advisory lock PostgreSQL para evitar dos despliegues concurrentes. Para revertir el primer esquema, ejecutar `001_auth_schema.down.sql` solo después de respaldar la base de Auth.

La columna `auth_roles.tenant_id` delimita los roles tenant. Los IDs de conjunto son referencias opacas; no existen claves foráneas a servicios externos. La correspondencia entre alcance del rol y tenant de la asignación se valida en la escritura y al calcular permisos.
