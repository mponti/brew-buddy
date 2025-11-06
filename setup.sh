#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e

echo "☕ Starting Brew Buddy environment setup..."

# 1. Create the local data directory
if [ ! -d "./local-data" ]; then
    mkdir -p ./local-data
    echo "✅ Created './local-data' directory for database storage."
else
    echo "ℹ️  './local-data' directory already exists."
fi

# Ensure the container (running as UID 1001) can write to this directory.
# standard 'chmod 777' is used here for maximum cross-platform compatibility
# on development machines without requiring sudo.
# intentional choice due to the data type and dev nature of the project.
chmod 777 ./local-data
echo "🔒 Set write permissions on './local-data' for Docker accessibility."

# 2. Create the configuration file from the example
if [ ! -f "./config.yaml" ]; then
    if [ -f "./config.example.yaml" ]; then
        cp config.example.yaml config.yaml
        echo "✅ Created 'config.yaml' from example template."
        echo "⚠️  ACTION REQUIRED: Please edit 'config.yaml' with your target URLs and selectors before running!"
    else
        echo "❌ Error: 'config.example.yaml' not found. Cannot create config file."
        exit 1
    fi
else
    echo "ℹ️  'config.yaml' already exists. Skipping overwrite to protect your settings."
fi

echo ""
echo "🎉 Brew Buddy setup complete! You are ready to build and run."
echo ""
echo "Next steps:"
echo "  1. Edit config.yaml with your real target data."
echo "  2. docker build -t brew-buddy ."
echo "  3. Run the container (see README.md for command)."
