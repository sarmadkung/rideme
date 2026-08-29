import { useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, Text, TextInput, View } from 'react-native';
import { tokens } from '@platform/ui';
import type { AuthActions, AuthState } from '../features/auth/useAuth';

/**
 * Phone-OTP sign-in (document 028).
 *
 * The screen holds only what the user is currently typing. Everything about
 * where the flow *is* comes from useAuth, so the two platforms cannot drift.
 */
export function LoginScreen({ auth }: { auth: AuthState & AuthActions }) {
  const [phone, setPhone] = useState('');
  const [code, setCode] = useState('');

  const onPhoneStage = auth.stage === 'phone';

  return (
    <View style={styles.screen} testID="login-screen">
      <Text style={styles.title}>RideMe</Text>
      <Text style={styles.subtitle}>
        {onPhoneStage ? 'Enter your phone number to continue.' : `We sent a code to ${auth.phone}.`}
      </Text>

      {onPhoneStage ? (
        <TextInput
          testID="phone-input"
          style={styles.input}
          value={phone}
          onChangeText={setPhone}
          placeholder="03001234567"
          placeholderTextColor={tokens.color.textMuted}
          keyboardType="phone-pad"
          autoComplete="tel"
          editable={!auth.pending}
        />
      ) : (
        <TextInput
          testID="code-input"
          style={styles.input}
          value={code}
          onChangeText={setCode}
          placeholder="6-digit code"
          placeholderTextColor={tokens.color.textMuted}
          keyboardType="number-pad"
          autoComplete="sms-otp"
          editable={!auth.pending}
        />
      )}

      {auth.error !== null && (
        <Text style={styles.error} testID="login-error">
          {auth.error}
        </Text>
      )}

      <Pressable
        testID="login-submit"
        style={({ pressed }) => [styles.button, pressed && styles.buttonPressed]}
        disabled={auth.pending}
        onPress={() => {
          if (onPhoneStage) void auth.requestCode(phone.trim());
          else void auth.submitCode(code.trim());
        }}
      >
        {auth.pending ? (
          <ActivityIndicator color={tokens.color.text} />
        ) : (
          <Text style={styles.buttonText}>{onPhoneStage ? 'Send code' : 'Verify'}</Text>
        )}
      </Pressable>

      {!onPhoneStage && (
        <Pressable testID="login-restart" onPress={auth.restart} disabled={auth.pending}>
          <Text style={styles.link}>Use a different number</Text>
        </Pressable>
      )}
    </View>
  );
}

const styles = StyleSheet.create({
  screen: {
    flex: 1,
    backgroundColor: tokens.color.background,
    padding: tokens.space.lg,
    justifyContent: 'center',
  },
  title: { color: tokens.color.text, fontSize: tokens.fontSize.xl, fontWeight: '600' },
  subtitle: {
    color: tokens.color.textMuted,
    fontSize: tokens.fontSize.md,
    marginTop: tokens.space.sm,
    marginBottom: tokens.space.lg,
  },
  input: {
    backgroundColor: tokens.color.surface,
    borderColor: tokens.color.border,
    borderWidth: 1,
    borderRadius: tokens.radius.md,
    color: tokens.color.text,
    fontSize: tokens.fontSize.lg,
    padding: tokens.space.md,
  },
  error: { color: tokens.color.danger, fontSize: tokens.fontSize.sm, marginTop: tokens.space.md },
  button: {
    backgroundColor: tokens.color.accent,
    borderRadius: tokens.radius.md,
    padding: tokens.space.md,
    alignItems: 'center',
    marginTop: tokens.space.lg,
  },
  buttonPressed: { opacity: 0.8 },
  buttonText: { color: tokens.color.text, fontSize: tokens.fontSize.md, fontWeight: '600' },
  link: {
    color: tokens.color.accent,
    fontSize: tokens.fontSize.sm,
    textAlign: 'center',
    marginTop: tokens.space.md,
  },
});
