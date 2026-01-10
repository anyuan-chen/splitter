"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import { api, TrackState } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import Link from "next/link";
import { ArrowLeft, Loader2, Music, Mic, Drum, Guitar, Layers } from "lucide-react";

const STEMS = [
  { name: "Original", file: "base.mp3", icon: Music, requiresDemucs: false },
  { name: "Vocals", file: "htdemucs/base/vocals.mp3", icon: Mic, requiresDemucs: true },
  { name: "Drums", file: "htdemucs/base/drums.mp3", icon: Drum, requiresDemucs: true },
  { name: "Bass", file: "htdemucs/base/bass.mp3", icon: Guitar, requiresDemucs: true },
  { name: "Other", file: "htdemucs/base/other.mp3", icon: Layers, requiresDemucs: true },
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

  const getStatusBadge = (status: string) => {
    switch (status) {
      case "completed":
        return <Badge variant="default" className="bg-green-600">Completed</Badge>;
      case "in_progress":
        return <Badge variant="secondary">In Progress</Badge>;
      case "failed":
        return <Badge variant="destructive">Failed</Badge>;
      default:
        return <Badge variant="outline">Pending</Badge>;
    }
  };

  return (
    <div className="container mx-auto py-10 px-4 min-h-screen">
      <div className="flex flex-col gap-8">
        {/* Header */}
        <div className="flex items-center gap-4">
          <Link href="/tracks">
            <Button variant="outline" size="icon">
              <ArrowLeft className="w-4 h-4" />
            </Button>
          </Link>
          <div>
            <h1 className="text-3xl font-bold">{track.name}</h1>
            <p className="text-muted-foreground">{track.artists}</p>
          </div>
        </div>

        {/* Status Cards */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-lg">Download Status</CardTitle>
            </CardHeader>
            <CardContent>
              {getStatusBadge(track.download_status)}
              {track.download_error && (
                <p className="text-sm text-red-500 mt-2">{track.download_error}</p>
              )}
            </CardContent>
          </Card>
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-lg">Demucs Status</CardTitle>
            </CardHeader>
            <CardContent>
              {getStatusBadge(track.demucs_status)}
              {track.demucs_error && (
                <p className="text-sm text-red-500 mt-2">{track.demucs_error}</p>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Audio Players */}
        <Card>
          <CardHeader>
            <CardTitle>Audio Tracks</CardTitle>
            <CardDescription>
              Listen to the original and separated stems
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6">
            {STEMS.map((stem) => {
              const isAvailable = stem.requiresDemucs 
                ? track.demucs_status === "completed"
                : track.download_status === "completed";
              
              const Icon = stem.icon;

              return (
                <div key={stem.name} className="flex flex-col gap-2">
                  <div className="flex items-center gap-2">
                    <Icon className="w-5 h-5 text-muted-foreground" />
                    <span className="font-medium">{stem.name}</span>
                    {!isAvailable && (
                      <Badge variant="outline" className="text-xs">Not Available</Badge>
                    )}
                  </div>
                  {isAvailable ? (
                    <audio
                      controls
                      className="w-full"
                      src={api.getAudioUrl(trackId, stem.file)}
                    >
                      Your browser does not support the audio element.
                    </audio>
                  ) : (
                    <div className="h-12 bg-muted rounded-md flex items-center justify-center text-muted-foreground text-sm">
                      {stem.requiresDemucs ? "Waiting for Demucs processing..." : "Waiting for download..."}
                    </div>
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
