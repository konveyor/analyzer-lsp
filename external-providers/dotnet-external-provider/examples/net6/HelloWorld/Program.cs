using System;

namespace HelloWorld
{
    class Program
    {
        public void NonPortableMethod()
        {
            Console.WriteLine("Hello World!");
        }

        static void Main(string[] args)
        {
            Program p = new Program();
            p.NonPortableMethod();
        }
    }
}
