# Using Teldrive with Media Servers (Plex/Jellyfin/Emby)

This guide shows the basic pattern for using Teldrive with Jellyfin, Plex, or Emby.

Finish the main Teldrive setup first.

## 1. Install the Docker volume plugin

Install the rclone Docker volume plugin:

```bash
docker plugin install ghcr.io/tgdrive/docker-volume-rclone --alias rclone --grant-all-permissions args="--allow-other" config=/etc/rclone cache=/var/cache
```

> [!NOTE]
> If you're using a different architecture, find all plugin tags [here](https://github.com/tgdrive/rclone/pkgs/container/docker-volume-rclone). Adjust the rclone cache and config directory paths accordingly.

## 2. Run Jellyfin

Create a Docker Compose file to run Jellyfin with access to your Teldrive storage:

::: code-group

```yml [docker-compose.yml]
services:
  jellyfin:
    image: jellyfin/jellyfin
    container_name: jellyfin
    volumes:
      - /path/to/config:/config
      - /path/to/cache:/cache
      - rclone:/media

volumes:
  rclone:
    external: true
```
:::

Plex and Emby can be configured the same way.

## 3. Important note about restart behavior

> [!IMPORTANT]
> - Don't use restart policies in compose files that use the rclone volume driver.
> - Start dependent containers only after the rclone volume exists.
> - If you automate startup, use a script like this:

```bash
#!/bin/bash
set -e

is_container_running() {
  local container_name=$1
  docker inspect -f '{{.State.Running}}' $container_name 2>/dev/null || echo "false"
}

POLL_INTERVAL=2

CONTAINERS=("teldrive")

for container in "${CONTAINERS[@]}"; do
  while [ "$(is_container_running $container)" != "true" ]; do
    echo "Waiting for container $container to be running..."
    sleep $POLL_INTERVAL
  done
  echo "Container $container is running."
done

VOLUME_NAME="rclone"

VOLUME_EXISTS=$(docker volume inspect $VOLUME_NAME > /dev/null 2>&1 && echo "yes" || echo "no")

# Create the volume if it does not exist
if [ "$VOLUME_EXISTS" == "no" ]; then
  echo "Volume $VOLUME_NAME does not exist. Creating volume..."
  docker volume create \
    --driver rclone \
    --opt remote="teldrive_remote_name:" \
    --opt vfs_cache_max_age=7720h \
    --opt vfs_read_chunk_streams=2 \
    --opt vfs_read_chunk_size=64M \
    --opt vfs_cache_max_size=300G \
    $VOLUME_NAME
  echo "Volume $VOLUME_NAME created successfully."
else
  echo "Volume $VOLUME_NAME already exists. Skipping creation."
fi

# Add other services here which depend on the rclone volume
cd /path/to/jellyfin/compose && docker compose up -d
```

If you do not need automated startup ordering, stop at the basic Jellyfin example above.
