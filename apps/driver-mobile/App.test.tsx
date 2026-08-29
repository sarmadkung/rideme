import { render, screen } from '@testing-library/react-native';
import App from './App';

describe('Driver shell', () => {
  it('says so rather than starting a shell that cannot reach anything', () => {
    // With no API base URL every screen behind the shell would show a network
    // error and none would say why.
    render(<App />);
    expect(screen.getByTestId('app-root')).toBeTruthy();
    expect(screen.getByTestId('app-env')).toHaveTextContent(/not configured/);
  });
});
