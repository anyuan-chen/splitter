"use client";

import {
	AlertCircle,
	ArrowLeft,
	CheckCircle2,
	Drum,
	Guitar,
	Layers,
	Loader2,
	Mic,
	Music,
} from "lucide-react";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useCallback, useEffect, useState } from "react";
import { AudioPlayer } from "@/components/audio-player";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card";
import { Progress } from "@/components/ui/progress";
import { api, type ProgressEvent, type TrackState } from "@/lib/api";

const STEMS = [
	{ name: "Original", file: "base.mp3", icon: Music, requiresDemucs: false },
	{
		name: "Vocals",
		file: "mdx_extra_q/base/vocals.wav",
		icon: Mic,
		requiresDemucs: true,
	},
	{
		name: "Drums",
		file: "mdx_extra_q/base/drums.wav",
		icon: Drum,
		requiresDemucs: true,
	},
	{
		name: "Bass",
		file: "mdx_extra_q/base/bass.wav",
		icon: Guitar,
		requiresDemucs: true,
	},
	{
		name: "Other",
		file: "mdx_extra_q/base/other.wav",
		icon: Layers,
		requiresDemucs: true,
	},
];

export default function TrackPage() {
	const params = useParams();
	const trackId = params.trackId as string;

	const [track, setTrack] = useState<TrackState | null>(null);
	const [loading, setLoading] = useState(true);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		if (!trackId) return;

		const fetchTrack = async () => {
			try {
				setLoading(true);
				const data = await api.getTrack(trackId);
				setTrack(data);
				setError(null);
			} catch (e) {
				console.error(e);
				setError("Failed to load track");
			} finally {
				setLoading(false);
			}
		};

		fetchTrack();
	}, [trackId]);

	const handleProgressEvent = useCallback(
		(event: ProgressEvent) => {
			if (event.track_id !== trackId) return;

			setTrack((currentTrack) => {
				if (!currentTrack) return null;

				const updatedTrack = { ...currentTrack };

				if (event.type === "download") {
					if (event.status === "downloading") {
						updatedTrack.download_status = "in_progress";
						updatedTrack.download_progress = event.progress;
					} else if (event.status === "completed") {
						updatedTrack.download_status = "completed";
						updatedTrack.download_progress = 100;
						updatedTrack.download_error = undefined;
					} else if (event.status === "failed") {
						updatedTrack.download_status = "failed";
						updatedTrack.download_error = event.error;
					}
				} else if (event.type === "demucs") {
					if (event.status === "pending") {
						updatedTrack.demucs_status = "in_progress";
						updatedTrack.demucs_progress = 0;
					} else if (event.status === "processing") {
						updatedTrack.demucs_status = "in_progress";
						updatedTrack.demucs_progress = event.progress;
					} else if (event.status === "completed") {
						updatedTrack.demucs_status = "completed";
						updatedTrack.demucs_progress = 100;
						updatedTrack.demucs_error = undefined;
					} else if (event.status === "failed") {
						updatedTrack.demucs_status = "failed";
						updatedTrack.demucs_error = event.error;
					}
				}

				return updatedTrack;
			});
		},
		[trackId],
	);

	useEffect(() => {
		if (loading || !track) return;

		const eventSource = api.createProgressSource();

		eventSource.onmessage = (event) => {
			try {
				const parsedEvent: ProgressEvent = JSON.parse(event.data);
				handleProgressEvent(parsedEvent);
			} catch (e) {
				console.error("Failed to parse progress event:", e);
			}
		};

		return () => {
			eventSource.close();
		};
	}, [loading, handleProgressEvent, track === null]);

	if (loading) {
		return (
			<div className="flex justify-center items-center h-screen">
				<Loader2 className="w-8 h-8 animate-spin text-muted-foreground" />
			</div>
		);
	}

	if (error || !track) {
		return (
			<div className="container mx-auto py-10 px-4">
				<div className="text-red-500 p-4 border border-red-200 rounded-md bg-red-50 dark:bg-red-900/10">
					{error || "Track not found"}
				</div>
				<Link href="/tracks" className="mt-4 inline-block">
					<Button variant="outline">
						<ArrowLeft className="w-4 h-4 mr-2" /> Back to Tracks
					</Button>
				</Link>
			</div>
		);
	}

	return (
		<div className="container mx-auto py-10 px-4 min-h-screen">
			<div className="flex flex-col gap-8">
				{/* Header */}
				<div className="flex items-center justify-between gap-4">
					<div className="flex items-center gap-4">
						<Link href="/tracks">
							<Button variant="outline" size="icon">
								<ArrowLeft className="w-4 h-4" />
							</Button>
						</Link>
						<div className="flex flex-col">
							<h1 className="text-xl font-mono text-muted-foreground">
								{track.name} <span className="mx-2">/</span> {track.artists}
							</h1>
						</div>
					</div>

					<div className="flex items-center gap-4">
						{(() => {
							const originalStem = STEMS.find((s) => s.name === "Original");
							if (!originalStem) return null;
							const isAvailable = track.download_status === "completed";
							const Icon = originalStem.icon;

							return (
								<div className="flex items-center gap-4 bg-muted/20 px-4 py-2 rounded-lg border border-zinc-100 dark:border-zinc-800/50">
									{isAvailable ? (
										<AudioPlayer
											src={api.getAudioUrl(trackId, originalStem.file)}
											className="bg-transparent border-none p-0 h-auto w-48"
										/>
									) : (
										<div className="text-[10px] italic text-muted-foreground">
											Waiting for download...
										</div>
									)}
								</div>
							);
						})()}
					</div>
				</div>

				{/* Audio Players */}
				<Card>
					<CardHeader>
						<CardTitle className="font-mono">Stems</CardTitle>
					</CardHeader>
					<CardContent className="space-y-1">
						{STEMS.filter((s) => s.name !== "Original").map((stem) => {
							const isAvailable = stem.requiresDemucs
								? track.demucs_status === "completed"
								: track.download_status === "completed";

							const Icon = stem.icon;

							return (
								<div
									key={stem.name}
									className="flex flex-row items-center gap-4 py-3 border-b last:border-0 border-zinc-100 dark:border-zinc-800/50"
								>
									<div className="flex items-center gap-3 w-32 shrink-0">
										<Icon className="w-4 h-4 text-muted-foreground" />
										<span className="text-sm font-medium">{stem.name}</span>
									</div>

									<div className="flex-1 min-w-0">
										{isAvailable ? (
											<AudioPlayer
												src={api.getAudioUrl(trackId, stem.file)}
												className="max-w-none w-full bg-transparent border-none p-0 h-auto"
											/>
										) : (
											<div className="h-8 bg-muted/30 rounded-md flex items-center px-3 text-muted-foreground text-xs italic border border-dashed border-muted-foreground/10">
												{stem.requiresDemucs
													? "Waiting for Demucs..."
													: "Waiting for download..."}
											</div>
										)}
									</div>

									{!isAvailable && (
										<Badge
											variant="outline"
											className="text-[10px] uppercase tracking-wider h-5 px-1.5 shrink-0 bg-transparent border-muted-foreground/20 text-muted-foreground"
										>
											Locked
										</Badge>
									)}
								</div>
							);
						})}
					</CardContent>
				</Card>
			</div>
		</div>
	);
}
