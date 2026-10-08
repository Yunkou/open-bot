import { Stack } from "expo-router";
import { HeroUINativeProvider } from "heroui-native";
import type { JSX } from "react";
import { GestureHandlerRootView } from "react-native-gesture-handler";

import { SessionProvider } from "@/providers/session";

import "../global.css";

export default function RootLayout(): JSX.Element {
  return (
    <GestureHandlerRootView style={{ flex: 1 }}>
      <HeroUINativeProvider>
        <SessionProvider>
          <Stack screenOptions={{ headerShown: false }}>
            <Stack.Screen name="login" />
            <Stack.Screen name="chats" />
          </Stack>
        </SessionProvider>
      </HeroUINativeProvider>
    </GestureHandlerRootView>
  );
}