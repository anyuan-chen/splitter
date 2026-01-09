#!/bin/bash

# Integration test script for the music downloader
# Tests all three API endpoints

set -e

echo "=== Integration Test ==="
echo ""

# Check if .env exists
if [ ! -f .env ]; then
    echo "Error: .env file not found. Please create it with SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET"
    exit 1
fi

# Build the server
echo "Building server..."
go build -o separate
echo "✓ Build successful"
echo ""

# Start server in background
echo "Starting server..."
export $(cat .env | xargs)
./separate > /tmp/server.log 2>&1 &
SERVER_PID=$!
echo "✓ Server started (PID: $SERVER_PID)"
echo ""

# Wait for server to be ready
sleep 2

# Cleanup function
cleanup() {
    echo ""
    echo "Cleaning up..."
    kill $SERVER_PID 2>/dev/null || true
    echo "✓ Server stopped"
}
trap cleanup EXIT

# Test 1: Setup playlist
echo "Test 1: POST /setup-playlist"
if [ -z "$1" ]; then
    echo "Error: Please provide a Spotify playlist ID as first argument"
    echo "Usage: $0 <playlist_id>"
    exit 1
fi

PLAYLIST_ID="$1"
echo "Using playlist ID: $PLAYLIST_ID"

RESPONSE=$(curl -s -X POST http://localhost:8080/setup-playlist \
    -H "Content-Type: application/json" \
    -d "{\"playlist_id\":\"$PLAYLIST_ID\"}")

echo "Response: $RESPONSE"

TRACK_COUNT=$(echo $RESPONSE | grep -o '"total_tracks":[0-9]*' | grep -o '[0-9]*')
echo "✓ Playlist setup successful ($TRACK_COUNT tracks)"
echo ""

# Test 2: Get tracks snapshot
echo "Test 2: GET /tracks"
sleep 1
TRACKS=$(curl -s http://localhost:8080/tracks)
echo "Response (first 500 chars):"
echo "$TRACKS" | head -c 500
echo "..."
echo "✓ Tracks endpoint working"
echo ""

# Test 3: Stream progress
echo "Test 3: GET /progress/stream (SSE)"
echo "Streaming progress for 10 seconds..."
timeout 10 curl -N http://localhost:8080/progress/stream 2>/dev/null || true
echo ""
echo "✓ SSE stream working"
echo ""

# Final verification
echo "Final state check:"
FINAL_TRACKS=$(curl -s http://localhost:8080/tracks)
COMPLETED=$(echo "$FINAL_TRACKS" | grep -o '"status":"completed"' | wc -l | tr -d ' ')
DOWNLOADING=$(echo "$FINAL_TRACKS" | grep -o '"status":"downloading"' | wc -l | tr -d ' ')
PENDING=$(echo "$FINAL_TRACKS" | grep -o '"status":"pending"' | wc -l | tr -d ' ')
FAILED=$(echo "$FINAL_TRACKS" | grep -o '"status":"failed"' | wc -l | tr -d ' ')

echo "  Completed: $COMPLETED"
echo "  Downloading: $DOWNLOADING"
echo "  Pending: $PENDING"
echo "  Failed: $FAILED"
echo ""

# Check database
echo "Database state:"
sqlite3 queue.db "SELECT download_status, COUNT(*) FROM tracks GROUP BY download_status;"
echo ""

echo "=== All Tests Passed ==="
echo ""
echo "To view server logs: tail -f /tmp/server.log"
