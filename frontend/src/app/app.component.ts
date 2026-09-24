import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';
import { HttpClient } from '@angular/common/http';

@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CommonModule],
  template: `
    <h1>Hello from Frontend</h1>
    <button (click)="sayHello()">Say hello to the backend</button>
    <p *ngIf="message">{{ message }}</p>
  `,
})
export class AppComponent {
  message = '';

  constructor(private http: HttpClient) {}

  sayHello(): void {
    // Relative path -- nginx.conf proxies /api/* to the backend, so the
    // browser only ever talks to its own origin. No CORS config needed
    // anywhere because of that.
    this.http.get<{ message: string }>('/api/hello').subscribe({
      next: (res) => (this.message = res.message),
      error: () => (this.message = 'Could not reach the backend.'),
    });
  }
}
