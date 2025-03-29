APP_NAME=k8s-platform
IMAGE_NAME=$(APP_NAME):latest

build:
	go build -o $(APP_NAME) main.go

run:
	go run main.go

test:
	go mod tidy
	go test ./... -v

kind-install:
	curl -Lo kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
	chmod +x kind
	sudo mv kind /usr/local/bin/kind

kind-create:
	kind create cluster --name $(APP_NAME)

docker:
	docker build -t $(IMAGE_NAME) .

kind-load:
	kind load docker-image $(IMAGE_NAME) --name $(APP_NAME)

helm-install:
	helm upgrade --install $(APP_NAME) ./chart --namespace=crossplane-system

argocd-install:
	kubectl create namespace argocd
	kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	kubectl wait --for=condition=available --timeout=180s deployment/argocd-server -n argocd
	kubectl apply -f apps/argo-app-bucket.yaml

crossplane-install:
	helm repo add crossplane-stable https://charts.crossplane.io/stable
	helm repo update
	helm install crossplane crossplane-stable/crossplane -n crossplane-system --create-namespace
	kubectl wait --for=condition=Available deployment/crossplane -n crossplane-system --timeout=120s
	kubectl get pods -n crossplane-system
	kubectl api-resources | grep crossplane
	kubectl apply -f infra/s3-provider.yaml 
	kubectl apply -f infra/dynamodb-provider.yaml
#   kubectl create secret generic aws-secret -n crossplane-system --from-file=creds=./aws-credentials.txt
	kubectl create secret generic my-secret --from-literal=aws_access_key_id=MOCK --from-literal=aws_secret_access_key=MOCK
	kubectl apply -f infra/provider-config.yaml

apply-resources:
	kubectl apply -f infra/dev-user.yaml
	kubectl auth can-i get buckets --as=dev-user --namespace=default
	kubectl apply -f infra/functions/patch-and-transform.yaml
	kubectl apply -f infra/nosql-xrd.yaml
	kubectl apply -f infra/nosql-composition.yaml
	kubectl apply -f infra/nosql-claim.yaml

helm-uninstall:
	helm uninstall $(APP_NAME) --namespace=crossplane-system

kind-delete:
	kind delete cluster --name $(APP_NAME)

crossplane-delete:
	kubectl delete -f infra/

deploy: kind-create crossplane-install docker kind-load helm-install

destroy: helm-uninstall kind-delete crossplane-delete