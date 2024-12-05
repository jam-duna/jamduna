# Import Blocks

You should _NOT_ supply a Dockerfile for your team

Build a Docker image:
```
TEAM_NAME=strongly-web3
docker build -t $TEAM_NAME/jam .
```

Be sure your binary can work with the public `genesis.json` and accept the required parameter of `seed`.

Launch JAM Testnet:
```
docker-compose up
```

## Publishing to a Registry

GCP:
```
PROJECT_ID=awesome-web3

# Step 1: Setup GCP
gcloud auth configure-docker

# Step 2: Tag the image
docker tag docker.io/$TEAM_NAME/jam gcr.io/$PROJECT_ID/$TEAM_NAME/jam

# Step 3: Push the image
docker push gcr.io/$PROJECT_ID/$TEAM_NAME/jam
```

