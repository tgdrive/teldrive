import type { Tags } from "@/types";
import { parseAudioMetadata } from "@/utils/tagparser";
import { create } from "zustand";

type AudioElem = HTMLAudioElement | null;

interface AudioMetadata {
  artist: string;
  title: string;
  cover: string;
}

interface AudioHandlers {
  nextItem: (type: string) => void;
  prevItem: (type: string) => void;
}

interface PlayerState {
  audio: AudioElem;
  isPlaying: boolean;
  duration: number;
  isLooping: boolean;
  isMuted: boolean;
  volume: number;
  isEnded: boolean;
  currentTime: number;
  metadata: AudioMetadata;
  metadataController: AbortController | null;
  error: string;
  handlers: AudioHandlers;
  actions: {
    seek: (value: number) => void;
    setVolume: (value: number) => void;
    togglePlay: () => void;
    toggleMute: () => void;
    toggleLooping: () => void;
    loadAudio: (url: string, name: string) => Promise<void>;
    set: (payload: Partial<PlayerState>) => void;
    reset: () => void;
    setCurrentTime: (value: number) => void;
    repeat: () => void;
    setHandlers: (handlers: AudioHandlers) => void;
  };
}

const DEFAULT_COVER_SVG = `data:image/svg+xml,%3Csvg viewBox='0 0 150 150' xmlns='http://www.w3.org/2000/svg'%3E%3Crect 
x='0' y='0' width='150' height='150' fill='lightgray' /%3E%3Csvg width='50' height='50' viewBox='0 0 24 24' fill='currentColor' 
x='50' y='50'%3E%3Cpath fill='white' fill-rule='evenodd' d='M12 2.25a.75.75 0 0 0-.75.75v11.26a4.25 4.25 0 1 0 1.486 2.888A1 
1 0 0 0 12.75 17V7.75H18a2.75 2.75 0 1 0 0-5.5zm.75 4H18a1.25 1.25 0 1 0 0-2.5h-5.25zm-4.25 8.5a2.75 2.75 0 1 0 0 5.5a2.75 
2.75 0 0 0 0-5.5' clip-rule='evenodd'/%3E%3C/svg%3E%3C/svg%3E`

const DEFAULT_METADATA: AudioMetadata = {
  artist: "Artista desconocido",
  title: "Título desconocido",
  cover: DEFAULT_COVER_SVG,
};

class AudioManager {
  private audio: HTMLAudioElement | null = null;
  private eventListeners: { event: string; handler: EventListener }[] = [];

  attachEventListener(event: string, handler: EventListener) {
    if (this.audio) {
      this.audio.addEventListener(event, handler);
      this.eventListeners.push({ event, handler });
    }
  }

  removeAllEventListeners() {
    if (this.audio) {
      this.eventListeners.forEach(({ event, handler }) => {
        this.audio?.removeEventListener(event, handler);
      });
      this.eventListeners = [];
    }
  }

  createAudio(): HTMLAudioElement {
    if (this.audio) {
      this.removeAllEventListeners();
      this.audio.pause();
      this.audio.src = "";
      this.audio.load();
    }
    this.audio = new Audio();
    return this.audio;
  }

  getAudio(): HTMLAudioElement | null {
    return this.audio;
  }

  cleanup() {
    if (this.audio) {
      this.removeAllEventListeners();
      this.audio.pause();
      this.audio.src = "";
      this.audio.load();
      this.audio = null;
    }
  }
}

export const useAudioStore = create<PlayerState>((set, get) => {
  const audioManager = new AudioManager();

  const updateMediaSession = (metadata: AudioMetadata) => {
    if ("mediaSession" in navigator && metadata.cover) {
      navigator.mediaSession.metadata = new MediaMetadata({
        title: metadata.title,
        artist: metadata.artist,
        artwork: [{ src: metadata.cover, type: "image/jpeg" }],
      });
    }
  };

  return {
    audio: null,
    isPlaying: false,
    duration: 0,
    isLooping: false,
    isMuted: false,
    volume: 1,
    isEnded: false,
    currentTime: 0,
    metadata: DEFAULT_METADATA,
    metadataController: null,
    error: "",
    handlers: {
      nextItem: () => {},
      prevItem: () => {},
    },

    actions: {
      loadAudio: async (url: string, name: string) => {
        const state = get();

        if (state.metadataController) {
          state.metadataController.abort();
        }

        const controller = new AbortController();
        set({ metadataController: controller });

        try {
          const audio = audioManager.createAudio();

          audioManager.attachEventListener("loadedmetadata", () => {
            set({ duration: Number.isFinite(audio.duration) ? audio.duration : 0 });
          });
          audioManager.attachEventListener("error", () => {
            set({ error: "No se pudo reproducir este formato. Prueba el modo compatible o descarga el archivo para abrirlo en VLC.", isPlaying: false });
          });

          audioManager.attachEventListener("play", () => {
            set({ isPlaying: true });
          });

          audioManager.attachEventListener("pause", () => {
            set({ isPlaying: false });
          });

          audioManager.attachEventListener("ended", () => {
            get().handlers.nextItem("audio");
            set({ isEnded: true, currentTime: 0 });
          });

          const previousCover = state.metadata.cover;
          if (previousCover.startsWith("blob:")) URL.revokeObjectURL(previousCover);
          audio.src = url;
          audio.volume = state.volume;
          audio.muted = state.isMuted;
          audio.loop = state.isLooping;
          audio.autoplay = false;
          set({ audio, isPlaying: false, isEnded: false, currentTime: 0, duration: 0, error: "", metadata: { ...DEFAULT_METADATA, title: name } });
          audio.load();
          // Metadata support must never block playback of other audio formats.
          const tags = url.includes("/api/media/") || url.includes("/media/") ? {} : await parseAudioMetadata(url, controller.signal).catch(() => ({}));
          if (controller.signal.aborted) return;
          const { artist, title, picture } = tags as Tags;

          const cover = picture ? URL.createObjectURL(picture) : DEFAULT_COVER_SVG;
          const metadata = {
            artist: artist || DEFAULT_METADATA.artist,
            title: title || name,
            cover,
          };

          updateMediaSession(metadata);

          set({
            audio,
            isPlaying: !audio.paused,
            isEnded: false,
            metadata,
            currentTime: 0,
            error: "",
          });
        } catch (error) {
          if ((error as Error).name !== "AbortError") {
            set({ error: "No se pudo cargar el audio. Inténtalo de nuevo." });
          }
        }
      },

      seek: (value: number) => {
        const audio = audioManager.getAudio();
        if (audio) {
          audio.currentTime = value;
          set({ currentTime: value });
        }
      },

      setVolume: (value: number) => {
        const audio = audioManager.getAudio();
        if (audio) {
          const normalizedVolume = Math.max(0, Math.min(1, value));
          audio.volume = normalizedVolume;
          set({ volume: normalizedVolume });
        }
      },

      setCurrentTime: (value: number) => set({ currentTime: value }),

      repeat: () => {
        const audio = audioManager.getAudio();
        if (audio) {
          audio.currentTime = 0;
          set({ currentTime: 0 });
        }
      },

      togglePlay: () => {
        const audio = audioManager.getAudio();
        if (audio) {
          if (get().isPlaying) audio.pause();
          else void audio.play().catch(() => set({ error: "No se pudo iniciar la reproducción. Prueba el modo compatible.", isPlaying: false }));
        }
      },

      toggleMute: () => {
        const audio = audioManager.getAudio();
        if (audio) {
          audio.muted = !audio.muted;
          set({ isMuted: audio.muted });
        }
      },

      toggleLooping: () => {
        const audio = audioManager.getAudio();
        if (audio) {
          audio.loop = !audio.loop;
          set({ isLooping: audio.loop });
        }
      },

      set: (payload: Partial<PlayerState>) => set(payload),

      reset: () => {
        const state = get();
        if (state.metadata.cover.startsWith("blob:")) URL.revokeObjectURL(state.metadata.cover);
        if (state.metadataController) {
          state.metadataController.abort();
        }

        audioManager.cleanup();

        set({
          audio: null,
          isPlaying: false,
          duration: 0,
          isLooping: false,
          isMuted: false,
          volume: 1,
          isEnded: false,
          currentTime: 0,
          metadata: DEFAULT_METADATA,
          metadataController: null,
          error: "",
        });
      },

      setHandlers: (handlers: AudioHandlers) => set({ handlers }),
    },
  };
});

export const audioActions = (state: PlayerState) => state.actions;
