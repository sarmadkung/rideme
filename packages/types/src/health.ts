export type HealthStatus = 'healthy' | 'degraded' | 'unhealthy';

export interface DependencyHealth {
  name: string;
  status: HealthStatus;
  latency_ms: number;
  error?: string | undefined;
}

export interface HealthResponse {
  status: HealthStatus;
  service: string;
  version: string;
  checked_at: string;
  dependencies: DependencyHealth[];
}
