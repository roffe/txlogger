Hi Joakim,

Thank you for contacting us. 
Please see the link and details below: 

https://plxdevices.freshdesk.com/support/solutions/articles/44002394240-imfd-serial-protocol-plx-app-018

The iMFD Digital output is: 

19200 Baud 
8 Data Bits 
1 Stop Bit 
No parity 

Packet format is as follows for the SM-AFR:

1) Start bit (0x80) 
2) Address bit MSB (0x00) for SM-AFR 
3) Address bit LSB (0x00) for SM-AFR 
4) Instance (0x00) if only one SM-AFR is connected 
5) Data MSB 
6) Data LSB 
7) Stop bit (0x40) 
(Packet set repeats exactly every 100mS) 


Interpreting the data bits is as follows. 

int datamsb; 
int datalsb; 
int data; 

datamsb = datamsb & 0x3F; //This ignores 2 most significant bits 
datalsb = datalsb & 0x3F; 

data = (datamsb xtagstartz< 6) | datalsb; //data is the AFR value

Best Regards,
Andrew Yanez
Customer Relations Specialist 
www.plxdevices.com