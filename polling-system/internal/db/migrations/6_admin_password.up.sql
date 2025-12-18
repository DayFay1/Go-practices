UPDATE users
SET password_hash = '$2a$10$8WevhHe/xtE9Ge7GBscwUOFgckDl.PLmIXpEyLjkMG4JTwO9PLk26'
WHERE email = 'admin@example.com'
  AND password_hash = '$2a$10$1FZFnKbLgn02k8/vQ5RqK.1D4fl2dSZ4fpV5GYVAiLhJxXYBf8LWe';
