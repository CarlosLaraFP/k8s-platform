APP_NAME=k8s-platform
IMAGE_NAME=claim-controller:latest

build:
	go build -o claim-controller main.go

run:
	go run main.go

test:
	go mod tidy
	go test ./... -v

terraform-apply:
	cd terraform && terraform init
	cd terraform && terraform plan
	cd terraform && terraform apply --auto-approve

kind-install:
	curl -Lo kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
	chmod +x kind
	sudo mv kind /usr/local/bin/kind

kind-create:
	kind create cluster --name $(APP_NAME)

crossplane-install:
	helm repo add crossplane-stable https://charts.crossplane.io/stable
	helm repo update
	helm install crossplane crossplane-stable/crossplane -n crossplane-system --create-namespace
	kubectl wait --for=condition=Available deployment/crossplane -n crossplane-system --timeout=120s
	kubectl get pods -n crossplane-system
	kubectl api-resources | grep crossplane
	kubectl apply -f infra/ec2-provider.yaml
	kubectl apply -f infra/s3-provider.yaml 
	kubectl apply -f infra/dynamodb-provider.yaml
	kubectl wait --for=condition=Healthy provider/provider-aws-dynamodb --timeout=180s
	kubectl wait --for=condition=Installed provider/provider-aws-dynamodb --timeout=180s

crossplane-provider:
	kubectl create secret generic aws-secret -n crossplane-system --from-file=creds=./aws-credentials.txt
	kubectl apply -f infra/provider-config.yaml

crossplane-provider-ci:
	kubectl create secret generic aws-secret -n crossplane-system --from-file=creds=./mock-aws-credentials.txt
	kubectl apply -f infra/provider-config.yaml

docker:
	docker build -t $(IMAGE_NAME) .

kind-load:
	kind load docker-image $(IMAGE_NAME) --name $(APP_NAME)

helm-install:
	helm upgrade --install $(APP_NAME) ./chart --namespace=crossplane-system

apply:
	kubectl apply -f infra/functions/patch-and-transform.yaml
	kubectl apply -f infra/storage-xrd.yaml
	kubectl apply -f infra/storage-composition.yaml
	kubectl apply -f infra/storage-claim.yaml
	kubectl apply -f infra/compute-xrd.yaml
	kubectl apply -f infra/compute-composition.yaml
	kubectl apply -f infra/compute-claim.yaml

argocd-install:
	kubectl create namespace argocd
	kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	kubectl wait --for=condition=available --timeout=180s deployment/argocd-server -n argocd
	kubectl apply -f infra/argocd-app.yaml

metrics-local:
	kubectl port-forward -n crossplane-system deployment/claim-controller 8080:8080
	curl -s http://localhost:8080/metrics | grep claims

helm-uninstall:
	helm uninstall $(APP_NAME) --namespace=crossplane-system

kind-delete:
	kind delete cluster --name $(APP_NAME)

unapply:
	kubectl delete -f infra/

terraform-helm-clean:
	cd terraform && terraform destroy -target helm_release.platform -auto-approve
	terraform apply -target helm_release.platform -auto-approve

terraform-destroy:
	cd terraform && terraform destroy -auto-approve

deploy-ci: kind-create crossplane-install crossplane-provider-ci docker kind-load helm-install

deploy: kind-create crossplane-install crossplane-provider docker kind-load helm-install

destroy: helm-uninstall kind-delete