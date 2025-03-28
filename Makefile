apply:
	kubectl apply -f infra/

argo-app:
	kubectl apply -f apps/argo-app-bucket.yaml

clean:
	kubectl delete -f infra/
