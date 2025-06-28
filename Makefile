APP_NAME=k8s-platform
IMAGE_NAME=claim-controller:latest
FUNCTION_NAME = function-docker-build
LOCAL_REGISTRY = localhost:5001

build:
	cd claim-controller && go build -o claim-controller main.go

run:
	cd claim-controller && go run .

test:
	cd claim-controller && go mod tidy
	cd claim-controller && go test ./... -v

terraform-apply-eks:
	cd infra-eks && terraform init
	cd infra-eks && terraform plan
	cd infra-eks && terraform apply --auto-approve

terraform-apply-k8s:
	cd infra-k8s && terraform init
	cd infra-k8s && terraform apply -target=null_resource.gateway_api --auto-approve
	cd infra-k8s && terraform plan
	cd infra-k8s && terraform apply --auto-approve

kind-install:
	curl -Lo kind https://kind.sigs.k8s.io/dl/v0.20.0/kind-linux-amd64
	chmod +x kind
	sudo mv kind /usr/local/bin/kind

kind-create:
	kind create cluster --name $(APP_NAME)

crossplane-install:
	helm repo add crossplane-stable https://charts.crossplane.io/stable
	helm repo update
	helm install crossplane crossplane-stable/crossplane -n crossplane-system --create-namespace \
	  --set image.repository=crossplane/crossplane \
	  --set image.tag=v1.19.1 \
	  --set securityContext.runAsUser=2000 \
	  --set securityContext.fsGroup=2000
	kubectl wait --for=condition=Available deployment/crossplane -n crossplane-system --timeout=120s
	kubectl get pods -n crossplane-system
	kubectl api-resources | grep crossplane
	kubectl apply --server-side -f infra/ec2-provider.yaml
	kubectl apply --server-side -f infra/s3-provider.yaml 
	kubectl apply --server-side -f infra/dynamodb-provider.yaml
	sleep 30
	kubectl get providers -o wide
	kubectl describe provider provider-aws-dynamodb
#	kubectl wait --for=condition=Healthy provider/provider-aws-dynamodb --timeout=300s
#	kubectl wait --for=condition=Healthy provider/provider-aws-ec2 --timeout=300s
#	kubectl wait --for=condition=Healthy provider/provider-aws-s3 --timeout=300s
#	kubectl wait --for=condition=Installed provider/provider-aws-dynamodb --timeout=200s

crossplane-provider:
	kubectl create secret generic aws-secret -n crossplane-system --from-file=creds=./aws-credentials.txt
	kubectl apply -f infra/provider-config.yaml

crossplane-provider-ci:
	kubectl create secret generic aws-secret -n crossplane-system --from-file=creds=./mock-aws-credentials.txt
	kubectl apply -f infra/provider-config.yaml

docker:
	docker build -t $(IMAGE_NAME) claim-controller/.
	docker build -t api-server:latest api-server/.
#	docker build -t function-docker-build:xpkg function-docker-build/.

kind-load:
	kind load docker-image $(IMAGE_NAME) --name $(APP_NAME)
	kind load docker-image api-server:latest --name $(APP_NAME)
#	kind load docker-image function-docker-build:xpkg --name k8s-platform

helm-install:
	helm upgrade --install claim-controller ./claim-controller-chart --namespace=crossplane-system
	helm upgrade --install api-server ./api-server-chart --namespace=crossplane-system

apply:
	kubectl apply -f infra/functions/patch-and-transform.yaml
	kubectl apply -f infra/functions/docker-build.yaml
	kubectl apply -f infra/storage-xrd.yaml
	kubectl apply -f infra/storage-composition.yaml
#	kubectl apply -f claims/storage-claim.yaml
	kubectl apply -f infra/compute-xrd.yaml
	kubectl apply -f infra/compute-composition.yaml
	kubectl apply -f infra/modeldeployment-xrd.yaml
	kubectl apply -f infra/modeldeployment-composition.yaml

	kubectl label node $(APP_NAME)-control-plane ingress-ready=true
	kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.10.0/deploy/static/provider/kind/deploy.yaml
	kubectl wait -n ingress-nginx --for=condition=Available deployment/ingress-nginx-controller --timeout=120s
	sleep 60
#	kubectl port-forward -n ingress-nginx service/ingress-nginx-controller 8080:80

argocd-install:
	kubectl create namespace argocd
	kubectl apply -n argocd -f https://raw.githubusercontent.com/argoproj/argo-cd/stable/manifests/install.yaml
	kubectl wait --for=condition=available --timeout=180s deployment/argocd-server -n argocd
	kubectl apply -f infra/argocd-project.yaml
	kubectl apply -f infra/argocd-app.yaml
	argocd app get k8s-platform
	argocd app sync k8s-platform

metrics-local:
	kubectl port-forward -n crossplane-system deployment/claim-controller 8080:8080
	curl -s http://localhost:8080/metrics | grep claims

control-plane-local:
	kubectl port-forward svc/api-server 8080:8080 -n crossplane-system
	curl http://localhost:8080/metrics

crossplane-package:
	docker build -t $(FUNCTION_NAME):xpkg $(FUNCTION_NAME)/.
	docker tag $(FUNCTION_NAME):xpkg $(LOCAL_REGISTRY)/$(FUNCTION_NAME):xpkg
	docker push $(LOCAL_REGISTRY)/$(FUNCTION_NAME):xpkg
	kind load docker-image $(LOCAL_REGISTRY)/$(FUNCTION_NAME):xpkg --name $(APP_NAME)
	kubectl apply -f infra/functions/docker-build.yaml

helm-uninstall:
	helm uninstall claim-controller --namespace=crossplane-system
	helm uninstall api-server --namespace=crossplane-system

kind-delete:
	kind delete cluster --name $(APP_NAME)

unapply:
	kubectl delete -f infra/

terraform-helm-clean:
	cd terraform && terraform destroy -target helm_release.platform -auto-approve
	terraform apply -target helm_release.platform -auto-approve

terraform-destroy-k8s:
	cd infra-k8s && terraform destroy -auto-approve

terraform-destroy-eks:
	cd infra-eks && terraform destroy -auto-approve

deploy-ci: kind-create crossplane-install crossplane-provider-ci docker kind-load apply helm-install

deploy: kind-create crossplane-install crossplane-provider docker kind-load apply helm-install

destroy: helm-uninstall kind-delete