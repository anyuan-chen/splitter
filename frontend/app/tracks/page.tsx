"use client";

import { Loader2 } from "lucide-react";
import { useState } from "react";
import { TracksTable } from "@/components/tracks-table";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useTracks } from "@/hooks/use-tracks";
import { api } from "@/lib/api";

export default function TracksPage() {
	const { tracks, loading, error, refresh } = useTracks();
	const [playlistId, setPlaylistId] = useState("");
	const [isSubmitting, setIsSubmitting] = useState(false);

	const handleSetupPlaylist = async () => {
		if (!playlistId) return;
		setIsSubmitting(true);
		try {
			await api.setupPlaylist(playlistId);
			setPlaylistId("");
			refresh();
		} catch (e) {
			console.error(e);
			alert("Failed to add playlist");
		} finally {
			setIsSubmitting(false);
		}
	};

	return (
		<div className="container mx-auto py-10 px-4 min-h-screen">
			<div className="flex flex-col gap-8">
				<div className="flex items-center justify-between">
					<h1 className="text-3xl font-bold">Tracks</h1>

					<div className="flex items-center gap-4">
						<div className="flex w-full max-w-sm items-center space-x-2">
							<Input
								type="text"
								placeholder="Spotify Playlist ID"
								value={playlistId}
								onChange={(e) => setPlaylistId(e.target.value)}
								disabled={isSubmitting}
							/>
							<Button
								onClick={handleSetupPlaylist}
								disabled={isSubmitting || !playlistId}
							>
								{isSubmitting ? (
									<Loader2 className="w-4 h-4 animate-spin" />
								) : (
									"Add Playlist"
								)}
							</Button>
						</div>
						<div className="text-sm text-muted-foreground whitespace-nowrap">
							{tracks.length} Tracks
						</div>
					</div>
				</div>

				{loading ? (
					<div className="flex justify-center items-center h-64">
						<Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
					</div>
				) : error ? (
					<div className="text-red-500 p-4 border border-red-200 rounded-md bg-red-50 dark:bg-red-900/10">
						Error: {error}
					</div>
				) : (
					<TracksTable tracks={tracks} />
				)}
			</div>
		</div>
	);
}
