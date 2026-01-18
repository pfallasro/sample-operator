.PHONY: install
install:
	kubectl apply -f crd/webapp-crd.yaml

.PHONY: uninstall
uninstall:
	kubectl delete -f crd/webapp-crd.yaml

.PHONY: run
run:
	go run main.go

.PHONY: deploy-example
deploy-example:
	kubectl apply -f examples/nginx-webapp.yaml

.PHONY: docker-build
docker-build:
	docker build -t webapp-operator:latest .

.PHONY: clean
clean:
	kubectl delete -f examples/ || true
	kubectl delete -f crd/webapp-crd.yaml || true
