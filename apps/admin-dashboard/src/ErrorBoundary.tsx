import { Component, type ErrorInfo, type ReactNode } from 'react';
import { tokens } from '@platform/ui';

interface Props {
  children: ReactNode;
}

interface State {
  error: Error | null;
}

export class ErrorBoundary extends Component<Props, State> {
  override state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  override componentDidCatch(error: Error, info: ErrorInfo): void {
    // Sentry is wired in the observability slice; the console is the Phase 1 sink.
    console.error('Unhandled render error', error, info.componentStack);
  }

  override render(): ReactNode {
    const { error } = this.state;
    if (!error) return this.props.children;

    return (
      <div role="alert" style={{ padding: tokens.space.lg, color: tokens.color.danger }}>
        <h1>Something went wrong</h1>
        <p>{error.message}</p>
      </div>
    );
  }
}
