from locust import HttpUser, task, between
import random
import json
from faker import Faker

class ChatUser(HttpUser):
    wait_time = between(1, 3)  # Random wait between 1-3 seconds
    host = "http://your-api-domain.com"  # Update with your API URL
    
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.faker = Faker()
        self.token = None
        self.receiver_id = "507f1f77bcf86cd799439011" 
        self.message_count = 0

    def on_start(self):
        """Login and get auth token before sending messages"""
        login_response = self.client.post("/api/auth/login", json={
            "email": "testuser@example.com",
            "password": "testpassword123"
        })
        self.token = login_response.json().get("token")
        

    @task(3)  
    def send_message(self):
        headers = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json"
        }
        
        payload = {
            "receiver_id": self.receiver_id,
            "content": f"{self.faker.sentence()} [LoadTest#{self.message_count}]",
            "content_type": random.choice(["text", "image", "video"]),
        }
        
        if random.random() < 0.2:
            payload["media_urls"] = [
                "https://example.com/media/image1.jpg",
                "https://example.com/media/image2.jpg"
            ]
        
        response = self.client.post(
            "/api/messages",
            headers=headers,
            json=payload,
            name="Send Message"
        )
        
        self.message_count += 1
        if response.status_code != 201:
            self.environment.events.request_failure.fire(
                request_type="POST",
                name="Send Message",
                response_time=response.elapsed.total_seconds() * 1000,
                exception=Exception(f"Status {response.status_code}"),
                response_length=len(response.content)
            )

    @task(1)
    def get_messages(self):
        headers = {
            "Authorization": f"Bearer {self.token}"
        }
        
        params = {
            "receiver_id": self.receiver_id,
            "limit": random.choice([10, 20, 50]),
            "page": random.randint(1, 5)
        }
        
        self.client.get(
            "/api/messages",
            headers=headers,
            params=params,
            name="Get Messages"
        )

    @task(1)
    def mark_as_seen(self):
        if self.message_count == 0:
            return
            
        headers = {
            "Authorization": f"Bearer {self.token}",
            "Content-Type": "application/json"
        }
        
        self.client.post(
            "/api/messages/seen",
            headers=headers,
            json=[str(self.message_count - i) for i in range(random.randint(1, 3))],
            name="Mark as Seen"
        )