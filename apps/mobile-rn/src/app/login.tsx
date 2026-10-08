import {
  Button,
  Card,
  Description,
  FieldError,
  Input,
  Label,
  Spinner,
  TextField,
  Typography,
} from "heroui-native";
import { useRouter } from "expo-router";
import type { JSX } from "react";
import { useState } from "react";
import { KeyboardAvoidingView, Platform, ScrollView, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import { API_BASE } from "@/api/config";
import { useSession } from "@/providers/session";

export default function LoginScreen(): JSX.Element {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const { signIn, signUp, user, restoring } = useSession();

  const [mode, setMode] = useState<"in" | "up">("in");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  if (!restoring && user) {
    router.replace("/chats");
  }

  async function submit(): Promise<void> {
    if (busy) return;
    if (!username.trim() || !password) {
      setError("请填写用户名和密码");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await (mode === "in" ? signIn(username.trim(), password) : signUp(username.trim(), password));
    } catch (err) {
      setError(err instanceof Error ? err.message : "请求失败，请检查 API 地址与网络");
    } finally {
      setBusy(false);
    }
  }

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-background"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
    >
      <ScrollView
        className="flex-1"
        contentContainerClassName="flex-grow justify-center px-5 py-10"
        keyboardShouldPersistTaps="handled"
      >
        <View className="gap-8" style={{ paddingBottom: insets.bottom }}>
          <View className="gap-2">
            <Typography.Heading type="h2">Open Bot</Typography.Heading>
            <Typography.Paragraph color="muted">
              登录后即可与你的助手对话
            </Typography.Paragraph>
          </View>

          <Card>
            <Card.Body className="gap-5">
              <TextField isInvalid={Boolean(error)}>
                <Label>
                  <Label.Text>用户名</Label.Text>
                </Label>
                <Input
                  value={username}
                  onChangeText={setUsername}
                  autoCapitalize="none"
                  autoCorrect={false}
                  placeholder="demo1"
                  returnKeyType="next"
                />
              </TextField>

              <TextField isInvalid={Boolean(error)}>
                <Label>
                  <Label.Text>密码</Label.Text>
                </Label>
                <Input
                  value={password}
                  onChangeText={setPassword}
                  secureTextEntry
                  placeholder="••••••••"
                  returnKeyType="go"
                  onSubmitEditing={() => void submit()}
                />
                {error ? <FieldError isInvalid>{error}</FieldError> : null}
              </TextField>

              <Button onPress={() => void submit()} isDisabled={busy} size="lg">
                {busy ? <Spinner size="sm" /> : null}
                <Button.Label>{mode === "in" ? "登录" : "注册并登录"}</Button.Label>
              </Button>

              <Button
                variant="ghost"
                size="sm"
                onPress={() => {
                  setMode(mode === "in" ? "up" : "in");
                  setError(null);
                }}
              >
                <Button.Label>
                  {mode === "in" ? "还没有账号？去注册" : "已有账号？去登录"}
                </Button.Label>
              </Button>
            </Card.Body>
          </Card>

          <Description className="text-center">API：{API_BASE}</Description>
        </View>
      </ScrollView>
    </KeyboardAvoidingView>
  );
}