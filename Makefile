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
	kubectl wait --for=condition=Healthy provider/provider-aws-dynamodb --timeout=180s
	kubectl wait --for=condition=Installed provider/provider-aws-dynamodb --timeout=180s
	kubectl create secret generic aws-secret -n crossplane-system --from-file=creds=./mock-aws-credentials.txt
	kubectl apply -f infra/provider-config.yaml

docker:
	docker build -t $(IMAGE_NAME) .

kind-load:
	kind load docker-image $(IMAGE_NAME) --name $(APP_NAME)

helm-install:
	helm upgrade --install $(APP_NAME) ./chart --namespace=crossplane-system

apply:
	kubectl apply -f infra/dev-user.yaml
	kubectl apply -f infra/dev-rolebinding.yaml
	kubectl auth can-i get storage --as=dev-user --namespace=default
	kubectl apply -f infra/functions/patch-and-transform.yaml
	kubectl apply -f infra/storage-xrd.yaml
	kubectl apply -f infra/storage-composition.yaml
	kubectl apply -f infra/storage-claim.yaml

metrics-local:
	kubectl port-forward -n crossplane-system deployment/k8s-platform 8080:8080
	curl -s http://localhost:8080/metrics | grep claims

helm-uninstall:
	helm uninstall $(APP_NAME) --namespace=crossplane-system

kind-delete:
	kind delete cluster --name $(APP_NAME)

unapply:
	kubectl delete -f infra/

deploy: kind-create crossplane-install docker kind-load helm-install

destroy: helm-uninstall kind-delete