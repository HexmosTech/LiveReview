/**
 * User notification utilities for new LiveReview signups
 * Handles Listmonk subscription and Discord notifications
 */

const LISTMONK_LIST_ID = "813662aa-1a51-404c-be60-981ead18d9fe";
const DISCORD_WEBHOOK_URL = "https://discord.com/api/webhooks/1394676585151332402/Gwp-Qvt-_0UHK8yVZ_6rPxRHm3Y0x_cdQICstDD7MQ2eBNyqJaatL-uyixTnFMy8KV_H";

// Consider users created within the last 5 minutes as "new" for notification purposes
const NEW_USER_THRESHOLD_MINUTES = 5;

/**
 * Send Discord notification for new user signup
 */
const sendDiscordNotification = async (name: string, email: string): Promise<void> => {
  try {
    if (!email) return; // don't spam if email missing
    
    const message = `🎉 New Cloud LiveReview User!\nName: ${name || "Not provided"}\nEmail: ${email || "Not provided"}\nTime: ${new Date().toISOString()}`;
    
    const response = await fetch(DISCORD_WEBHOOK_URL, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ content: message })
    });
    
    if (!response.ok) {
      throw new Error(`Discord webhook failed: ${response.status}`);
    }
    
    console.info("[LiveReview] Discord notification sent");
  } catch (err) {
    console.error("[LiveReview] Discord notification error", err);
  }
};

/**
 * Check if user is newly created (within threshold)
 */
const isNewUser = (createdAt: string): boolean => {
  try {
    const userCreatedTime = new Date(createdAt);
    const now = new Date();
    const diffMinutes = (now.getTime() - userCreatedTime.getTime()) / (1000 * 60);
    
    return diffMinutes <= NEW_USER_THRESHOLD_MINUTES;
  } catch (err) {
    console.error("[LiveReview] Failed to parse user creation date:", err);
    return false;
  }
};

/**
 * Subscribe user to Listmonk mailing list
 */
const subscribeToListmonk = async (
  email: string,
  name: string
): Promise<void> => {
  try {
    console.info("[LiveReview] Attempting Listmonk subscription for:", email);
    
    const body = new URLSearchParams({
      email: email,
      name: name,
      l: LISTMONK_LIST_ID,
      nonce: ""
    });
    
    const response = await fetch("https://lm.hexmos.com/subscription/form", {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: body.toString()
    });
    
    if (!response.ok) {
      throw new Error(`Listmonk response ${response.status}`);
    }
    
    console.info("[LiveReview] Listmonk subscription success");
  } catch (err) {
    console.warn("[LiveReview] Listmonk subscription failed (non-critical):", err);
    throw err;
  }
};

/**
 * Handle user login notification (should be called after successful login)
 * Only sends notifications for newly created users (within threshold)
 * Only runs in cloud deployments
 */
export const handleUserLoginNotification = async (
  email: string,
  firstName: string,
  lastName: string,
  createdAt: string
): Promise<void> => {
  // Only run in cloud deployments
  const { isCloudMode } = await import('./deploymentMode');
  if (!isCloudMode()) {
    console.info("[LiveReview] Not in cloud mode, skipping notifications");
    return;
  }
  
  if (!email) {
    console.warn("[LiveReview] No email provided for user notification");
    return;
  }
  
  // Only notify for new users
  if (!isNewUser(createdAt)) {
    console.info("[LiveReview] Existing user, skipping notification:", email);
    return;
  }
  
  console.info("[LiveReview] New user detected, sending notifications:", email);
  
  const name = [firstName, lastName].filter(Boolean).join(' ') || email.split('@')[0];
  
  try {
    // Subscribe to Listmonk
    await subscribeToListmonk(email, name);
    
    // Send Discord notification (non-blocking)
    sendDiscordNotification(name, email).catch(discordErr => {
      console.warn("[LiveReview] Discord notification failed (non-critical):", discordErr);
    });
  } catch (err) {
    console.warn("[LiveReview] User notification failed (non-critical):", err);
  }
};
