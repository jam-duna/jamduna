
# JAM Testnet with Docker + Docker-Compose

This builds a JAM Docker for your team and launches a JAM Testnet 

Build a Docker image:
```
docker build -t colorfulnotion/jam .
```


Launch JAM Testnet:
```
docker-compose up
```

## Publishing to GCP

This pushes the resulting image to GCP 

```
# Step 1: Setup GCP
gcloud auth configure-docker

# Step 2: Tag the image
docker tag docker.io/colorfulnotion/jam gcr.io/awesome-web3/jam-duna/jam

# Step 3: Push the image
docker push gcr.io/awesome-web3/jam-duna/jam
```

You could push it to other container registries
