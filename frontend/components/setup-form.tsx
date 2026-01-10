"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { api } from "@/lib/api";

export function SetupForm() {
	const [url, setUrl] = useState("");
	const [isLoading, setIsLoading] = useState(false);
	const [message, setMessage] = useState<{
		type: "success" | "error";
		text: string;
	} | null>(null);

	const extractPlaylistId = (input: string): string | null => {
		try {
			// Handle full URL
			if (input.includes("spotify.com")) {
				const urlObj = new URL(input);
				const pathSegments = urlObj.pathname.split("/");
				// Expected format: /playlist/{id}
				const playlistIndex = pathSegments.indexOf("playlist");
				if (playlistIndex !== -1 && playlistIndex + 1 < pathSegments.length) {
					return pathSegments[playlistIndex + 1];
				}
			}
			// Handle raw ID (basic check)
			if (/^[a-zA-Z0-9]{22}$/.test(input)) {
				return input;
			}
			return null;
		} catch (e) {
			return null;
		}
	};

	const handleSubmit = async (e: React.FormEvent) => {
		e.preventDefault();
		setMessage(null);
		setIsLoading(true);

		const playlistId = extractPlaylistId(url);
		if (!playlistId) {
			setMessage({ type: "error", text: "Invalid Spotify Playlist URL or ID" });
			setIsLoading(false);
			return;
		}

		try {
			const response = await api.setupPlaylist(playlistId);
			setMessage({
				type: "success",
				text: `Setup started for "${response.playlist_name}" with ${response.total_tracks} tracks.`,
			});
			setUrl("");
		} catch (error) {
			setMessage({
				type: "error",
				text:
					error instanceof Error ? error.message : "Failed to setup playlist",
			});
		} finally {
			setIsLoading(false);
		}
	};

	return (
		<Card className="w-full max-w-md mx-auto">
			<CardHeader>
				<CardTitle>Setup Playlist</CardTitle>
				<CardDescription>
					Enter a Spotify playlist URL to begin.
				</CardDescription>
			</CardHeader>
			<form onSubmit={handleSubmit}>
				<CardContent className="space-y-4">
					<div className="space-y-2">
						<Label htmlFor="url">Spotify Playlist URL</Label>
						<Input
							id="url"
							placeholder="https://open.spotify.com/playlist/..."
							value={url}
							onChange={(e) => setUrl(e.target.value)}
							disabled={isLoading}
						/>
					</div>
					{message && (
						<div
							className={`text-sm p-3 rounded-md ${
								message.type === "success"
									? "bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-400"
									: "bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-400"
							}`}
						>
							{message.text}
						</div>
					)}
				</CardContent>
				<CardFooter>
					<Button type="submit" className="w-full" disabled={isLoading || !url}>
						{isLoading ? "Setting up..." : "Start Download"}
					</Button>
				</CardFooter>
			</form>
		</Card>
	);
}
