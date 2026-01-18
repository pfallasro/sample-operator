#!/bin/bash

# WebApp Operator Quick Setup Script
# This script helps you get started with the operator example

set -e

echo "🚀 WebApp Operator Setup"
echo "========================"
echo ""

# Check prerequisites
echo "Checking prerequisites..."

if ! command -v kubectl &> /dev/null; then
    echo "❌ kubectl not found. Please install kubectl first."
    exit 1
fi
echo "✅ kubectl found"

if ! command -v go &> /dev/null; then
    echo "❌ Go not found. Please install Go 1.21+ first."
    exit 1
fi
echo "✅ Go found ($(go version))"

# Check if connected to a cluster
if ! kubectl cluster-info &> /dev/null; then
    echo "❌ Not connected to a Kubernetes cluster."
    echo "   Please configure kubectl to connect to a cluster (minikube, kind, etc.)"
    exit 1
fi
echo "✅ Connected to cluster: $(kubectl config current-context)"
echo ""

# Ask user what they want to do
echo "What would you like to do?"
echo "1) Install CRD only"
echo "2) Install CRD and run operator locally"
echo "3) Install CRD and deploy example WebApp"
echo "4) Full demo (install everything)"
echo "5) Clean up (remove all resources)"
echo ""
read -p "Enter choice [1-5]: " choice

case $choice in
    1)
        echo ""
        echo "📦 Installing CRD..."
        kubectl apply -f crd/webapp-crd.yaml
        echo "✅ CRD installed successfully!"
        echo ""
        echo "Verify with: kubectl get crd webapps.example.com"
        ;;
    2)
        echo ""
        echo "📦 Installing CRD..."
        kubectl apply -f crd/webapp-crd.yaml
        echo "✅ CRD installed!"
        echo ""
        echo "📥 Downloading Go dependencies..."
        go mod download
        echo "✅ Dependencies ready!"
        echo ""
        echo "🏃 Starting operator..."
        echo "   (Press Ctrl+C to stop)"
        echo ""
        go run main.go
        ;;
    3)
        echo ""
        echo "📦 Installing CRD..."
        kubectl apply -f crd/webapp-crd.yaml
        echo "✅ CRD installed!"
        echo ""
        echo "⏳ Waiting 2 seconds for CRD to be ready..."
        sleep 2
        echo ""
        echo "🚀 Creating example WebApp (nginx-app)..."
        kubectl apply -f examples/nginx-webapp.yaml
        echo "✅ WebApp created!"
        echo ""
        echo "📊 Current resources:"
        kubectl get webapps,deployments,services -l managed-by=webapp-operator
        echo ""
        echo "⚠️  Note: You need to run the operator for it to work!"
        echo "   Run: go run main.go"
        ;;
    4)
        echo ""
        echo "🎬 Starting full demo..."
        echo ""
        echo "📦 Installing CRD..."
        kubectl apply -f crd/webapp-crd.yaml
        echo "✅ CRD installed!"
        echo ""
        echo "📥 Downloading Go dependencies..."
        go mod download
        echo "✅ Dependencies ready!"
        echo ""
        echo "⏳ Waiting 2 seconds for CRD to be ready..."
        sleep 2
        echo ""
        echo "🚀 Creating example WebApp..."
        kubectl apply -f examples/nginx-webapp.yaml
        echo "✅ WebApp created!"
        echo ""
        echo "📊 Initial state:"
        kubectl get webapps
        echo ""
        echo "🏃 Starting operator..."
        echo "   Watch it create Deployment and Service automatically!"
        echo "   (Press Ctrl+C to stop)"
        echo ""
        go run main.go
        ;;
    5)
        echo ""
        echo "🧹 Cleaning up..."
        echo ""
        echo "Deleting WebApp resources..."
        kubectl delete -f examples/ --ignore-not-found=true
        echo ""
        echo "Deleting CRD (this also deletes all WebApp custom resources)..."
        kubectl delete -f crd/webapp-crd.yaml --ignore-not-found=true
        echo ""
        echo "✅ Cleanup complete!"
        echo ""
        echo "Remaining deployments and services will be garbage collected by Kubernetes."
        ;;
    *)
        echo "Invalid choice. Exiting."
        exit 1
        ;;
esac

echo ""
echo "📚 For more information, see README.md"
