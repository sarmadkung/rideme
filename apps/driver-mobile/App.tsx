import { StatusBar } from 'expo-status-bar';
import { StyleSheet, Text, View } from 'react-native';
import { tokens } from '@platform/ui';
import { loadEnvOrNull } from './src/env';

const env = loadEnvOrNull(process.env as Record<string, string | undefined>);

/**
 * Placeholder shell. It proves the Expo/React Native toolchain and workspace
 * package resolution — nothing more. Navigation, authentication, maps and the
 * booking flow arrive with their own slices.
 */
export default function App() {
  return (
    <View style={styles.container} testID="app-root">
      <Text style={styles.title}>RideMe Driver</Text>
      <Text style={styles.subtitle}>Driver shell — no product functionality yet.</Text>
      <Text style={styles.meta} testID="app-env">
        {env ? `${env.appEnv} · ${env.apiBaseUrl}` : 'environment not configured'}
      </Text>
      <StatusBar style="light" />
    </View>
  );
}

const styles = StyleSheet.create({
  container: {
    flex: 1,
    backgroundColor: tokens.color.background,
    alignItems: 'center',
    justifyContent: 'center',
    padding: tokens.space.lg,
  },
  title: {
    color: tokens.color.text,
    fontSize: tokens.fontSize.xl,
    fontWeight: '600',
  },
  subtitle: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.md,
    marginTop: tokens.space.sm,
  },
  meta: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.sm,
    marginTop: tokens.space.md,
  },
});
