# Docker Build Instructions

To build the Rollups Node Docker image, run the following command in the build directory.

```
docker buildx bake --load
```

Alternatively, you can build the image using the following command.

```
docker compose -f node-compose.yml build
```

And then run the following command to start the node with its dependecies.

```
docker compose -f node-compose.yml -f deps-compose.yml up
```
