# Device API Documentation

## เพิ่มอุปกรณ์ (Add Device)

### Endpoint
```
POST /api/devices
```

### Headers
```
Authorization: Bearer <your_jwt_token>
Content-Type: application/json
```

### Request Body
```json
{
  "name": "MacBook Pro 16-inch 2023",
  "type": "MacBook",
  "price": 500.00,
  "description": "MacBook Pro 16-inch M2 Max, 32GB RAM, 1TB SSD - สภาพใหม่ ใช้งานน้อย",
  "condition": "like-new",
  "imageUrl": "https://example.com/images/macbook-pro.jpg"
}
```

### Field Descriptions
- **name** (required): ชื่ออุปกรณ์
- **type** (required): ประเภทอุปกรณ์ - ต้องเป็นค่าใดค่าหนึ่งจาก:
  - `Notebook`
  - `MacBook`
  - `Other`
- **price** (required): ราคาเช่าต่อวัน (ต้องมากกว่า 0)
- **description** (optional): รายละเอียดสินค้า - สามารถใส่ข้อมูลสเปค, สภาพการใช้งาน
- **condition** (optional): สภาพอุปกรณ์ - ต้องเป็นค่าใดค่าหนึ่งจาก:
  - `new` - ใหม่
  - `like-new` - เหมือนใหม่
  - `good` - ดี (default)
  - `fair` - พอใช้
  - `poor` - เก่า
- **imageUrl** (optional): URL รูปภาพอุปกรณ์

### Success Response (201 Created)
```json
{
  "success": true,
  "message": "Device created successfully",
  "data": {
    "deviceNo": 123
  }
}
```

### Error Responses

#### 400 Bad Request - Missing Required Fields
```json
{
  "success": false,
  "message": "Name, type, and price are required",
  "error": ""
}
```

#### 400 Bad Request - Invalid Device Type
```json
{
  "success": false,
  "message": "Invalid device type",
  "error": "sql: no rows in result set"
}
```

#### 401 Unauthorized
```json
{
  "success": false,
  "message": "Unauthorized",
  "error": ""
}
```

---

## ดูอุปกรณ์ของตัวเอง (Get My Devices)

### Endpoint
```
GET /api/devices/my
```

### Headers
```
Authorization: Bearer <your_jwt_token>
```

### Success Response (200 OK)
```json
{
  "success": true,
  "message": "Devices retrieved successfully",
  "data": {
    "data": [
      {
        "deviceNo": 123,
        "name": "MacBook Pro 16-inch 2023",
        "description": "MacBook Pro 16-inch M2 Max, 32GB RAM, 1TB SSD",
        "price": 500.00,
        "type": "MacBook",
        "rating": 4.5,
        "status": "available",
        "condition": "like-new",
        "imageUrl": "https://example.com/images/macbook-pro.jpg",
        "createdAt": "2026-02-17T10:30:00Z"
      }
    ]
  }
}
```

---

## ดูอุปกรณ์ทั้งหมด (Browse All Devices)

### Endpoint
```
GET /api/devices/browse
```

### Query Parameters
- **type** (optional): กรองตามประเภท (`Notebook`, `MacBook`, `Other`, `all`)
- **status** (optional): กรองตามสถานะ (`available`, `rented`)

### Example
```
GET /api/devices/browse?type=MacBook&status=available
```

### Success Response (200 OK)
```json
{
  "success": true,
  "message": "Devices retrieved successfully",
  "data": {
    "data": [
      {
        "deviceNo": 123,
        "name": "MacBook Pro 16-inch 2023",
        "description": "MacBook Pro 16-inch M2 Max, 32GB RAM, 1TB SSD",
        "price": 500.00,
        "type": "MacBook",
        "rating": 4.5,
        "status": "available",
        "condition": "like-new",
        "imageUrl": "https://example.com/images/macbook-pro.jpg",
        "ownerEmail": "user@email.kmitl.ac.th",
        "createdAt": "2026-02-17T10:30:00Z"
      }
    ]
  }
}
```

---

## ดูรายละเอียดอุปกรณ์ (Get Device Details)

### Endpoint
```
GET /api/devices/{deviceNo}
```

### Headers
```
Authorization: Bearer <your_jwt_token>
```

### Example
```
GET /api/devices/123
```

### Success Response (200 OK)
```json
{
  "success": true,
  "message": "Device retrieved successfully",
  "data": {
    "deviceNo": 123,
    "name": "MacBook Pro 16-inch 2023",
    "description": "MacBook Pro 16-inch M2 Max, 32GB RAM, 1TB SSD",
    "price": 500.00,
    "type": "MacBook",
    "rating": 4.5,
    "status": "available",
    "condition": "like-new",
    "imageUrl": "https://example.com/images/macbook-pro.jpg",
    "ownerEmail": "user@email.kmitl.ac.th",
    "createdAt": "2026-02-17T10:30:00Z"
  }
}
```

---

## ลบอุปกรณ์ (Delete Device)

### Endpoint
```
DELETE /api/devices/{deviceNo}
```

### Headers
```
Authorization: Bearer <your_jwt_token>
```

### Success Response (200 OK)
```json
{
  "success": true,
  "message": "Device deleted successfully",
  "data": null
}
```

---

## วิธีรัน Migration Database

เพื่อเพิ่ม Column `Condition` ใน Database:

```bash
cd noteletwebservice-development
psql -U postgres -d notelet_db -f update_device_schema.sql
```

หรือถ้าใช้ Docker:

```bash
docker exec -i <postgres_container_name> psql -U postgres -d notelet_db < update_device_schema.sql
```

---

## ตัวอย่างการใช้งานด้วย JavaScript (Frontend)

### เพิ่มอุปกรณ์
```javascript
async function addDevice(deviceData) {
  const token = localStorage.getItem('accessToken');
  
  const response = await fetch('http://localhost:8080/api/devices', {
    method: 'POST',
    headers: {
      'Authorization': `Bearer ${token}`,
      'Content-Type': 'application/json'
    },
    body: JSON.stringify({
      name: deviceData.name,
      type: deviceData.type,
      price: parseFloat(deviceData.price),
      description: deviceData.description,
      condition: deviceData.condition || 'good',
      imageUrl: deviceData.imageUrl
    })
  });
  
  const result = await response.json();
  
  if (result.success) {
    console.log('Device created with ID:', result.data.deviceNo);
    return result.data.deviceNo;
  } else {
    throw new Error(result.message);
  }
}

// ตัวอย่างการเรียกใช้
addDevice({
  name: 'MacBook Air M2',
  type: 'MacBook',
  price: 350,
  description: 'MacBook Air M2 2023, 16GB RAM, 512GB SSD - สภาพดีมาก',
  condition: 'like-new',
  imageUrl: 'https://example.com/macbook-air.jpg'
});
```

### ดึงข้อมูลอุปกรณ์ทั้งหมด
```javascript
async function getAllDevices(type = 'all', status = 'available') {
  const response = await fetch(
    `http://localhost:8080/api/devices/browse?type=${type}&status=${status}`
  );
  
  const result = await response.json();
  
  if (result.success) {
    return result.data.data;
  } else {
    throw new Error(result.message);
  }
}

// ตัวอย่างการเรียกใช้
const devices = await getAllDevices('MacBook', 'available');
console.log(devices);
```
