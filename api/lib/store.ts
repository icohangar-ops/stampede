export interface Store {
  get(key: string): Promise<string | null>;
  set(key: string, value: string, ttlSeconds: number): Promise<void>;
}

type Entry = { value: string; exp: number };

export function memoryStore(): Store {
  const map = new Map<string, Entry>();
  return {
    async get(key) {
      const entry = map.get(key);
      if (!entry) return null;
      if (entry.exp <= Date.now()) {
        map.delete(key);
        return null;
      }
      return entry.value;
    },
    async set(key, value, ttlSeconds) {
      map.set(key, { value, exp: Date.now() + ttlSeconds * 1000 });
    },
  };
}

// On Vercel, Runtime Cache is shared across instances in a region and needs no secret.
// Off the platform, or if the cache call fails, the in-memory map is the store.
export function runtimeStore(): Store {
  const mem = memoryStore();
  if (process.env.VERCEL !== "1") return mem;
  return {
    async get(key) {
      const local = await mem.get(key);
      if (local != null) return local;
      const remote = await remoteGet(key);
      if (remote != null) {
        await mem.set(key, remote, 60);
        return remote;
      }
      return null;
    },
    async set(key, value, ttlSeconds) {
      await mem.set(key, value, ttlSeconds);
      await remoteSet(key, value, ttlSeconds);
    },
  };
}

async function remoteGet(key: string): Promise<string | null> {
  try {
    const { getCache } = await import("@vercel/functions");
    const value = await getCache({ namespace: "stampede" }).get(key);
    return typeof value === "string" ? value : null;
  } catch (err) {
    console.error("runtime cache get failed", err);
    return null;
  }
}

async function remoteSet(key: string, value: string, ttlSeconds: number): Promise<void> {
  try {
    const { getCache } = await import("@vercel/functions");
    await getCache({ namespace: "stampede" }).set(key, value, { ttl: ttlSeconds, tags: ["stampede"] });
  } catch (err) {
    console.error("runtime cache set failed", err);
  }
}
