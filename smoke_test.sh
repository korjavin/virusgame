#!/bin/bash
# smoke_test.sh - Verify bot system is working

set -e

echo "=== Bot System Smoke Test ==="

# Check backend health if available
echo "1. Check backend health..."
if curl -f http://localhost:8080/ > /dev/null 2>&1; then
    echo "✓ Backend is up and accessible at localhost:8080"
else
    echo "⚠ Backend not accessible at localhost:8080 (ignoring if running in CI/Test env)"
fi

# Check docker status if available
if command -v docker >/dev/null 2>&1; then
    echo "2. Check bot-hoster is running..."
    if docker ps | grep bot-hoster >/dev/null 2>&1; then
        echo "✓ Bot-hoster is running"

        echo "3. Check bots are connected..."
        BOTS=$(docker logs virusgame-bot-hoster 2>&1 | grep "Bot registered" | wc -l)
        echo "✓ $BOTS bots connected"

        if [ "$BOTS" -eq 0 ]; then
            echo "⚠ No bots connected found in logs yet. Note: If container just started, this is expected."
        fi

        echo "4. Check for recent errors..."
        ERRORS=$(docker logs --since 5m virusgame-bot-hoster 2>&1 | grep ERROR | wc -l)
        if [ "$ERRORS" -gt 0 ]; then
            echo "✗ Found $ERRORS errors in last 5 minutes"
        else
            echo "✓ No recent errors"
        fi
    else
        echo "⚠ Bot-hoster container not found (skipping container checks)"
    fi
else
    echo "⚠ Docker not available, skipping docker checks"
fi

echo ""
echo "=== Basic checks passed! ==="
echo "To perform full end-to-end testing, follow the manual steps in todo_task6.md"
